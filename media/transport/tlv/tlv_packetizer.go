package tlv

import (
	"github.com/eluv-io/common-go/media/transport/rtp"
	"github.com/eluv-io/common-go/util/byteutil"
	"github.com/eluv-io/common-go/util/sliceutil"
	"github.com/eluv-io/errors-go"
)

type options struct {
	valid            func(typ byte) bool
	recoverTsPadding func(typ byte) bool
}

type Option func(*options)

func OptValid(valid ...byte) Option {
	return func(o *options) {
		o.valid = func(typ byte) bool {
			return sliceutil.Contains(valid, typ)
		}
	}
}

func OptRecoverTsPadding(rec byte) Option {
	return func(o *options) {
		o.recoverTsPadding = func(typ byte) bool {
			return typ == rec
		}
	}
}

// NewTlvPacketizer creates a new TlvPacketizer instance. Use Options to configure valid TLV types and recovery
// behavior. maxPacketSize is the maximum allowed packet size. If a packet with a larger size is encountered, an error
// will be returned.
func NewTlvPacketizer(maxPacketSize uint16, opts ...Option) *Packetizer {
	o := options{
		valid: func(typ byte) bool {
			return true
		},
		recoverTsPadding: func(typ byte) bool {
			return false
		},
	}
	for _, opt := range opts {
		opt(&o)
	}

	//  buffer capacity: 1 byte for type, 2 bytes for length, maxPacketSize bytes for payload
	bufCap := 1 + 2 + int(maxPacketSize)
	return &Packetizer{
		maxPacketSize: maxPacketSize,
		buf:           byteutil.NewRingBuffer(bufCap),
		pkt:           make([]byte, bufCap), // if maxPacketSize is < 3 (!?), we can still parse the type and length
		options:       o,
	}
}

// Packetizer is a packetizer for type-length-value encoded packets.
type Packetizer struct {
	options       options
	maxPacketSize uint16 // the maximum packet size

	buf       *byteutil.RingBuffer // internal ring buffer for packetization
	pkt       []byte               // the next packet to be returned
	remaining []byte               // the remaining bytes from the last Write call that did not fit into the ring buffer

	// parse state
	haveTL     bool // true if the type and length fields have been parsed
	nextLength int  // the size of the next packet parsed from the TLV stream

	consumed int // number of bytes consumed by the last call to Next()
}

func (p *Packetizer) Write(bts []byte) {
	written := p.buf.Write(bts)
	p.remaining = bts[written:]
}

func (p *Packetizer) Next() ([]byte, error) {

	// fillMin fills the buffer with at least minBytesNeeded bytes from the p.remaining. It returns true if the buffer
	// is filled with the bytes needed, false otherwise.
	fillMin := func(minBytesNeeded int) bool {
		if p.buf.Len() >= minBytesNeeded {
			return true
		}
		p.Write(p.remaining) // no-op if remaining is empty...
		return p.buf.Len() >= minBytesNeeded
	}

	var typ byte
	var length uint16

	for {
		if !p.haveTL {
			if !fillMin(3) {
				return nil, nil
			}
			_ = p.buf.Read(p.pkt[:3])
			typ, length = ParseTlvHeader([3]byte(p.pkt[:3]))
			if !p.options.valid(typ) {
				return nil, errors.NoTrace("TlvPacketizer.Next", errors.K.Invalid,
					"reason", "invalid TLV type",
					"type", typ)
			}
			if length > p.maxPacketSize {
				return nil, errors.NoTrace("TlvPacketizer.Next", errors.K.Invalid,
					"max_packet_size", p.maxPacketSize,
					"actual_size", p.nextLength)
			}
			p.nextLength = int(length)
			p.haveTL = true
			p.consumed += 3
		}

		if !fillMin(p.nextLength) {
			return nil, nil
		}

		// read the next packet
		read := p.buf.Read(p.pkt[:p.nextLength])
		if read != p.nextLength {
			panic(
				errors.E("tlvPacketizer.Next", errors.K.Invalid,
					"reason", "buffer read invariant violation",
					"expected", p.nextLength,
					"actual", read,
				),
			)
		}
		p.consumed += p.nextLength

		// reset state
		p.haveTL = false

		if p.options.recoverTsPadding(typ) {
			recovered, err := rtp.RecoverTsPadding(p.pkt, read)
			if err == nil {
				// adapt next length to the recovered packet size, since that is what we will return
				p.nextLength = len(recovered)
			}
			return recovered, err
		}

		return p.pkt[:read], nil
	}
}

func (p *Packetizer) TargetPacketSize() int {
	return p.nextLength
}

// Consumed returns the number of bytes consumed to produce the last packet returned by Next(). Only call after Next()
// returns a non-nil packet.
func (p *Packetizer) Consumed() int {
	return p.consumed
}

func ParseTlvHeader(bts [3]byte) (typ byte, len uint16) {
	typ = bts[0]
	len = uint16(bts[1])<<8 | uint16(bts[2])
	return
}
