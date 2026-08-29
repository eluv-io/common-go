package pktpool

import (
	"encoding/binary"
	"io"
	"sync"
	"time"

	"github.com/Comcast/gots/v2/packet"
	"github.com/pion/rtp"

	"github.com/eluv-io/common-go/media/tlv/tlv"
	"github.com/eluv-io/errors-go"
)

// NewPacket creates a Packet with wrapCap bytes of headroom before its payload area and cap bytes of payload capacity.
// The headroom is used by WrapTlv to prepend protocol headers without reallocating or copying the payload.
func NewPacket(wrapCap, cap int) *Packet {
	p := &Packet{
		buf:        make([]byte, wrapCap+cap),
		initialOff: wrapCap,
	}
	return p.reset()
}

// Packet is a wrapper around a byte slice that provides lazy decoding of TLV, RTP and MPEG-TS and encoding of TLV.
//
// A packet maintains two independent views into its internal buffer:
//
//   - The full packet extent (off/len), exposed via the Data field and written by Write. From/FromReader define this
//     extent on input; WrapTlv grows it by prepending a TLV header.
//   - The lazy-decode cursor (decOff/decLen), which starts at the full extent and is advanced past each protocol layer
//     header/footer as that layer is decoded (TLV → RTP → MPEG-TS). Decoding never mutates Data, so Data always
//     reflects the complete packet regardless of which layers have been parsed.
//
// # Decoding protocol layers
//
// A Packet is just bytes; it does not know its own encapsulation. Which layers are present (e.g. TLV-wrapped RTP
// carrying MPEG-TS, or raw RTP) is knowledge that belongs to the caller, who must drive decoding accordingly. The
// layer accessors — Tlv, Rtp and Ts — are not independent: each one decodes the next layer starting at the current
// cursor position and then advances the cursor past that layer's header (and any footer/padding). The caller must
// therefore:
//
//   - Call the accessors from the outermost present layer inward, in encapsulation order (TLV before RTP before TS).
//   - Call an accessor only for layers that are actually present. To decode raw RTP, call Rtp directly (do not call
//     Tlv); to decode the MPEG-TS carried by RTP, call Rtp first, then Ts.
//
// Decoding a layer that is not actually at the cursor (e.g. calling Rtp on data that still starts with a TLV header,
// or calling Ts before Rtp) parses garbage and typically returns an error — it is a caller error, not something the
// Packet can detect or prevent.
//
// Each layer is parsed at most once; repeated accessor calls return the cached, already-decoded layer. The layer
// accessors are safe for concurrent use on a packet shared via the pool's reference counting. Methods that load or
// mutate the packet contents (From, FromReader and WrapTlv) must not run concurrently with each other or with
// accessors.
//
// Concurrency caveat: while the accessors are individually race-free, decoding distinct layers concurrently is
// nondeterministic. The accessors share a single forward-only cursor, and the first one to acquire the lock advances
// it, so e.g. concurrent Tlv and Rtp calls on a TLV-wrapped packet may, depending on scheduling, decode RTP from the
// wrong offset or fail the ordering guard. The contract that layers be decoded outermost-inward is therefore a
// per-sequence requirement: have a single goroutine decode the layers it needs (typically the producer, before
// sharing), then let consumers concurrently re-read the cached layers.
type Packet struct {
	Data []byte // the full packet data (a slice of buf): buf[off:off+len]

	// ReceivedAt records when the packet was received, for timestamp tracking. Reset on borrow, and by From/FromReader
	// as part of loading new data. FromReader re-stamps it itself (as of its Read call), since it performs the read.
	// From does not read from the network itself, so callers loading a datagram already read elsewhere must set
	// ReceivedAt explicitly after calling From (see e.g. mpegtsInputHandler.Read in avpipe).
	ReceivedAt time.Time

	buf        []byte // the internal buffer. Immutable (is never re-assigned to a subslice).
	initialOff int    // the initial offset into buf (== configured wrapCap)

	// full packet extent within buf
	off int // start offset of the full packet within buf (decreases when WrapTlv prepends a header)
	len int // length of the full packet

	// lazy decoding cursor: starts at the full packet extent and advances past each decoded protocol layer
	decOff int // current start offset into buf marking remaining undecoded data
	decLen int // current len of remaining undecoded data

	// mu guards lazy decoding (the cursor and the protocol-layer parse state) so that a packet shared between multiple
	// consumers via the pool's reference counting can be decoded concurrently without data races.
	mu sync.Mutex

	// lastRank is the rank of the most recently decoded protocol layer (-1 if none yet), used to reject decoding an
	// outer layer after an inner one has already consumed bytes. See decodeInOrder.
	lastRank int

	// protocol layers
	tlv TlvPacket
	rtp RtpPacket
	ts  TsPacket
}

// Protocol layer ranks, in encapsulation (outermost-to-innermost) order. Layers must be decoded in non-decreasing rank
// order; the cursor only moves forward, so decoding an outer layer after an inner one has consumed bytes is always a
// caller error.
const (
	rankTlv = iota
	rankRtp
	rankTs
)

func (p *Packet) init() {
	// A pristine packet is required on borrow per the pool's ResourceFactory contract. The buffer contents are not
	// zeroed (callers must not read beyond Data), but all extent/cursor/layer state is reset.
	p.reset()
}

func (p *Packet) reset() *Packet {
	p.resetForLoad()
	return p
}

// resetForLoad restores the packet to a pristine, empty state ready to be loaded: the full-packet extent and decode
// cursor are cleared (Data becomes nil), the offset is moved back to its initial value (undoing any head room consumed
// by WrapTlv), and all decoded protocol layers are invalidated. Clearing the extent here means a load that fails (From
// overflow) or reads nothing (FromReader with n == 0) leaves the packet empty rather than exposing stale data from a
// previous use. It must be called before writing new data at p.off in From/FromReader.
func (p *Packet) resetForLoad() {
	p.Data = nil
	p.ReceivedAt = time.Time{}
	p.off = p.initialOff
	p.len = 0
	p.decOff = p.initialOff
	p.decLen = 0
	p.lastRank = -1
	p.rtp.reset()
	p.tlv.reset()
	p.ts.reset()
}

// setExtent records the full packet extent [off, off+length) and keeps Data and the decode cursor in sync with it.
func (p *Packet) setExtent(length int) {
	p.len = length
	p.Data = p.buf[p.off : p.off+p.len]
	p.decOff = p.off
	p.decLen = p.len
}

// FromReader fills the packet from a single Read of the given reader and sets its ReceivedAt timestamp.
//
// It performs exactly one Read into the buffer's remaining capacity (len(buf)-off) and treats whatever that single
// Read returns as one complete packet. This makes it suitable only for message-oriented readers that preserve packet
// boundaries and return exactly one message per Read — e.g. UDP sockets or SRT connections in message mode (see
// srtConfig.MessageAPI). It is NOT suitable for stream-oriented readers (TCP, files, pipes), where a Read may return a
// partial packet or coalesce several: FromReader does not loop to fill the buffer and has no framing, so it would
// produce truncated or merged packets. Use a packetizer (e.g. media/tlv, media/mpegts) to frame a byte stream and feed
// the resulting packets via From instead.
//
// The extent (and Data) is updated only when n > 0. The reader's error is returned verbatim, including io.EOF; callers
// must check it. A datagram larger than the remaining capacity is truncated to that capacity by the underlying reader
// (the surplus is typically discarded by the OS for UDP).
func (p *Packet) FromReader(reader io.Reader) error {
	p.resetForLoad()
	n, err := reader.Read(p.buf[p.off:])
	if n > 0 {
		p.setExtent(n)
		p.ReceivedAt = time.Now()
	}
	return err
}

// From copies the given bytes into the packet's buffer. It returns an error if the data does not fit into the buffer's
// remaining capacity (would otherwise be silently truncated).
//
// From is self-contained: it resets the packet's load position and decode state first, so it is safe to reuse a packet
// (including one previously modified by WrapTlv) without going back through the pool.
//
// Note: From does not set the packet's ReceivedAt timestamp - set it outside if needed!
func (p *Packet) From(bts []byte) error {
	p.resetForLoad()
	if len(bts) > len(p.buf)-p.off {
		return errors.NoTrace("Packet.From", errors.K.Invalid,
			"reason", "data too large for buffer",
			"data_len", len(bts),
			"capacity", len(p.buf)-p.off)
	}
	p.setExtent(copy(p.buf[p.off:], bts))
	return nil
}

// decodeInOrder decodes the layer at the given rank from the current cursor position, enforcing that layers are decoded
// in non-decreasing rank order (see the Packet type doc). It advances the cursor past the decoded layer's head/tail
// bytes on success. Must be called with mu held and only when the layer has not yet been parsed.
func (p *Packet) decodeInOrder(rank int, decode func([]byte) (int, int, error)) error {
	if rank <= p.lastRank {
		return errors.NoTrace("pktpool.Packet", errors.K.Invalid,
			"reason", "protocol layer decoded out of order",
			"rank", rank,
			"last_rank", p.lastRank)
	}
	head, tail, err := decode(p.buf[p.decOff : p.decOff+p.decLen])
	if err != nil {
		return err
	}
	p.decOff += head
	p.decLen -= head + tail
	p.lastRank = rank
	return nil
}

// Tlv returns the TLV packet wrapping the remaining data, parsing it on first access (advancing the decode cursor past
// the TLV header) and caching the result. See the Packet type doc for the layer-ordering contract.
func (p *Packet) Tlv() (*TlvPacket, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tlv.parsed {
		return &p.tlv, nil
	}
	return &p.tlv, p.decodeInOrder(rankTlv, p.tlv.decode)
}

// Rtp returns the RTP packet contained in this Packet, parsing it on first access (advancing the decode cursor past the
// RTP header and trailing padding) and caching the result. See the Packet type doc for the layer-ordering contract.
func (p *Packet) Rtp() (*RtpPacket, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rtp.parsed {
		return &p.rtp, nil
	}
	return &p.rtp, p.decodeInOrder(rankRtp, p.rtp.decode)
}

// Ts returns the MPEG-TS packets contained in the remaining data, parsing them on first access and caching the result.
// See the Packet type doc for the layer-ordering contract.
func (p *Packet) Ts() (*TsPacket, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ts.parsed {
		return &p.ts, nil
	}
	return &p.ts, p.decodeInOrder(rankTs, p.ts.decode)
}

// Write writes the packet's full Data extent to writer. It returns io.ErrShortWrite if writer accepts fewer bytes than
// len(Data) without returning an error.
func (p *Packet) Write(writer io.Writer) error {
	n, err := writer.Write(p.Data)
	if err != nil {
		return err
	}
	if n != len(p.Data) {
		return io.ErrShortWrite
	}
	return nil
}

// WrapTlv prepends a TLV header (type + 2-byte big-endian length) to the full packet extent, growing Data accordingly.
// The decode cursor is left untouched, so it continues to point at the wrapped (inner) payload, but p.Data is updated
// to include the TLV header. Returns io.ErrShortBuffer if there is not enough head room (configured via wrapCap) to
// prepend the 3-byte header, and errors.K.Invalid if the payload is too large to encode in the 16-bit length field.
//
// If a TLV header is already present (the TLV layer was decoded or previously wrapped), WrapTlv only updates the type
// byte in place — both the in-memory layer and the serialized buffer — without prepending a second header.
//
// Also see FrameTlv for a zero-copy framing alternative that does not mutate the packet and is safe for concurrent
// readers.
func (p *Packet) WrapTlv(typ byte) error {
	if p.tlv.parsed {
		// retype the existing header in place; p.buf[p.off] is the TLV type byte for both a wrapped and a decoded TLV
		// (the full-packet offset points at the TLV header in both cases), and p.tlv.data aliases it.
		p.buf[p.off] = typ
		p.tlv.typ = typ
		return nil
	}
	if p.len > 0xffff {
		return errors.NoTrace("Packet.WrapTlv", errors.K.Invalid,
			"reason", "payload too large for TLV length field", "len", p.len)
	}
	if p.off < 3 {
		return io.ErrShortBuffer
	}
	size := uint16(p.len)
	p.off -= 3
	p.buf[p.off] = typ
	binary.BigEndian.PutUint16(p.buf[p.off+1:p.off+3], size)
	p.len += 3
	p.Data = p.buf[p.off : p.off+p.len]
	p.tlv.parsed = true
	p.tlv.data = p.Data
	p.tlv.Payload = p.Data[3:]
	p.tlv.typ = typ
	p.tlv.size = size
	return nil
}

// FrameTlv returns this packet's data wrapped in a TLV header (a type byte plus a big-endian 16-bit length), with the
// optional prefix inserted between the header and the payload - e.g. FrameTlv(atsType, timestamp) yields
// [header][timestamp][payload]. The framing is written into the reserved head room (configured via wrapCap) and the
// returned slice aliases the packet buffer, so it is valid only until the packet is reloaded or released.
//
// Unlike WrapTlv, FrameTlv does NOT mutate the packet: Data, the full-packet extent and the decode cursor are all left
// unchanged. Because it never touches Data, FrameTlv is safe to call on a packet concurrently read (via Data) by other
// consumers sharing it through the pool's reference counting - provided no other goroutine writes the same head room.
// This makes it the tool for framing output on a fanned-out packet, where WrapTlv's in-place Data reassignment would
// race with the other readers.
//
// Returns io.ErrShortBuffer if the head room cannot hold the header plus prefix, and errors.K.Invalid if the framed
// value (prefix + payload) exceeds the 16-bit TLV length field.
func (p *Packet) FrameTlv(typ byte, prefix []byte) ([]byte, error) {
	valueLen := len(prefix) + p.len
	if valueLen > 0xffff {
		return nil, errors.NoTrace("Packet.FrameTlv", errors.K.Invalid,
			"reason", "value too large for TLV length field", "len", valueLen)
	}
	head := 3 + len(prefix)
	if p.off < head {
		return nil, io.ErrShortBuffer
	}
	start := p.off - head
	p.buf[start] = typ
	binary.BigEndian.PutUint16(p.buf[start+1:start+3], uint16(valueLen))
	copy(p.buf[start+3:p.off], prefix)
	return p.buf[start : p.off+p.len], nil
}

// ---------------------------------------------------------------------------------------------------------------------

type basePacket struct {
	parsed  bool
	data    []byte // the full layer data (header + payload [+ footer])
	Payload []byte // the payload data (full packet minus header/footer)
}

// Data returns the bytes of this protocol layer (its exact extent depends on the layer — see the layer type's doc). It
// aliases the owning Packet's buffer and is valid only until that Packet is released or reloaded. Only valid after the
// layer has been parsed.
func (p *basePacket) Data() []byte {
	return p.data
}

func (p *basePacket) reset() {
	p.parsed = false
	p.data = nil
	p.Payload = nil
}

// ---------------------------------------------------------------------------------------------------------------------

// TlvPacket is a decoded TLV layer. Data contains the TLV header and value; Payload contains only the value bytes.
type TlvPacket struct {
	basePacket
	typ  byte
	size uint16
}

// Type returns the TLV type byte. Only valid after the packet has been parsed.
func (p *TlvPacket) Type() byte {
	return p.typ
}

// Size returns the TLV payload size in bytes. Only valid after the packet has been parsed.
func (p *TlvPacket) Size() uint16 {
	return p.size
}

func (p *TlvPacket) reset() {
	p.basePacket.reset()
}

func (p *TlvPacket) decode(data []byte) (int, int, error) {
	p.data = data
	if len(p.data) < 3 {
		return 0, 0, io.ErrShortBuffer
	}
	p.typ, p.size = tlv.ParseHeader([3]byte(p.data[:3]))
	if 3+int(p.size) > len(p.data) {
		return 0, 0, errors.NoTrace("TlvPacket.decode", errors.K.Invalid, "reason", "data too short")
	}
	tail := len(p.data) - 3 - int(p.size)
	p.data = p.data[:3+int(p.size)]
	p.Payload = p.data[3:]
	p.parsed = true
	return 3, tail, nil
}

// ---------------------------------------------------------------------------------------------------------------------

// RtpPacket is a decoded RTP layer. Data contains the RTP packet bytes; Payload aliases the RTP payload bytes.
type RtpPacket struct {
	basePacket
	pkt rtp.Packet
}

// Packet returns the decoded pion RTP packet. The returned packet aliases the owning Packet's internal buffer and is
// valid only until the owning Packet is released back to the pool or reloaded.
func (p *RtpPacket) Packet() *rtp.Packet {
	return &p.pkt
}

func (p *RtpPacket) reset() {
	p.basePacket.reset()
}

func (p *RtpPacket) decode(data []byte) (int, int, error) {
	p.data = data
	// unmarshaling multiple times into the same packet instance is safe (see unit tests in pion/rtp)
	err := p.pkt.Unmarshal(p.data)
	if err != nil {
		return 0, 0, err
	}
	p.Payload = p.pkt.Payload
	p.parsed = true
	// Derive the header length (cursor advance) from where pion actually placed the payload rather than from
	// Header.MarshalSize(): data == [header][payload][padding] and p.pkt.Payload aliases data, so the header length is
	// exactly len(data) - len(payload) - padding. MarshalSize() recomputes the header from the parsed extensions and
	// can come out smaller than the bytes pion consumed when the extension block carried extra padding, which would
	// leave the cursor short of the real payload.
	padding := int(p.pkt.Header.PaddingSize)
	head := len(data) - len(p.pkt.Payload) - padding
	return head, padding, nil
}

// ---------------------------------------------------------------------------------------------------------------------

// TsPacket is a decoded MPEG-TS layer. Data and Payload contain the full TS byte stream.
type TsPacket struct {
	basePacket
	// packets holds pointers into the underlying buffer (zero-copy). packet.Packet is [188]byte, so storing values
	// would copy 188 bytes per TS packet; the slice-to-array-pointer conversion below aliases the buffer instead.
	packets []*packet.Packet
}

// Packets returns the MPEG-TS packets contained in this layer. Each packet aliases the underlying buffer (zero-copy);
// the returned slice is valid only until the owning Packet is released back to the pool. Only valid after parsing.
func (p *TsPacket) Packets() []*packet.Packet {
	return p.packets
}

func (p *TsPacket) reset() {
	p.basePacket.reset()
	p.packets = p.packets[:0]
}

func (p *TsPacket) decode(data []byte) (int, int, error) {
	if len(data)%packet.PacketSize != 0 {
		return 0, 0, errors.NoTrace("TsPacket.decode", errors.K.Invalid,
			"reason", "data length not multiple of TS packet size",
			"length", len(data))
	}
	num := len(data) / packet.PacketSize

	if cap(p.packets) < num {
		p.packets = make([]*packet.Packet, num)
	} else {
		p.packets = p.packets[:num]
	}

	for i := range num {
		p.packets[i] = (*packet.Packet)(data[i*packet.PacketSize : (i+1)*packet.PacketSize])
	}

	p.data = data
	p.Payload = data
	p.parsed = true
	return num * packet.PacketSize, 0, nil
}
