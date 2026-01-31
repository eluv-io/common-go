package rtp

import (
	"bytes"
	"encoding/hex"
	"strings"

	ts "github.com/Comcast/gots/v2/packet"
	"github.com/pion/rtp"

	"github.com/eluv-io/errors-go"
	elog "github.com/eluv-io/log-go"
)

var log = elog.Get("/eluvio/media/transport/rtp")

// the log for periodic stats from the StreamTracker
var statsLog = elog.Get("/eluvio/media/transport/rtp/stats")

// ParsePacket parses the given RTP packet. Returns an error if the packet is invalid.
func ParsePacket(packet []byte) (*rtp.Packet, error) {
	pkt := rtp.Packet{}
	err := pkt.Unmarshal(packet)
	if err != nil {
		return nil, errors.NoTrace("rtp.ParsePacket", errors.K.Invalid, err, "reason", "failed to unmarshal RTP packet")
	}
	return &pkt, nil
}

// StripHeaderFull parses the full RTP header and strips the RTP header from the given packet. Returns the payload or an error if the byte slice does not
// start with an RTP header.
func StripHeaderFull(packet []byte) ([]byte, error) {
	pkt := rtp.Packet{}
	err := pkt.Unmarshal(packet)
	if err != nil {
		return nil, errors.NoTrace("StripHeader", errors.K.Invalid, err, "reason", "failed to unmarshal RTP packet")
	}
	return pkt.Payload, nil
}

// StripHeader parses minimally and strips the RTP header from the given packet. Returns the payload or an error if the byte slice does not
// start with an RTP header.
func StripHeader(bts []byte) ([]byte, error) {
	if len(bts) < 64 || bts[0]&0xC0 != 0x80 {
		return nil, errors.NoTrace("Packet is control or not RTP", errors.K.Invalid)
	}

	var h rtp.Header
	if _, err := h.Unmarshal(bts); err != nil {
		return nil, err
	}
	hdrLen := h.MarshalSize()
	if hdrLen > len(bts) {
		return nil, errors.NoTrace("Packet is too short")
	}
	if h.Padding {
		pad := int(bts[len(bts)-1])
		if hdrLen+pad > len(bts) {
			return nil, errors.NoTrace("Packet padding is bd")
		}
		return bts[hdrLen : len(bts)-pad], nil
	}
	return bts[hdrLen:], nil
}

// RemoveTsPadding removes the padding payload of TS padding packets within the given RTP packet. The removal is
// performed in-place. The TS header of padding packets is preserved. Returns the RTP packet with the stripped TS
// packets or an error if the packet is invalid.
func RemoveTsPadding(pkt []byte) ([]byte, error) {
	rtpPacket, err := ParsePacket(pkt)
	if err != nil {
		return nil, err
	}
	hdrLen := rtpPacket.Header.MarshalSize()
	for offset := hdrLen; offset+188 <= len(pkt); offset += 188 {
		tsPkt := ts.Packet(pkt[offset : offset+188])
		if tsPkt.IsNull() {
			// a padding packet: strip the payload.
			// TS header: 4 bytes, payload: 184 bytes
			copy(pkt[offset+4:], pkt[offset+188:]) // preserve padding packet header
			pkt = pkt[:len(pkt)-184]               // adjust datagram size...
			offset -= 184                          // ... and offset to account for the removed payload
		}
	}
	return pkt, nil
}

var padding = bytes.Repeat([]byte{0xFF}, 184)

// RecoverTsPadding recovers the padding of TS packets within the given RTP packet. Recovery is performed in-place. The
// caller has to ensure that the packet is large enough to hold the recovered padding, otherwise an error is returned.
// Returns the recovered packet or an error if the packet is invalid.
func RecoverTsPadding(pkt []byte, size int) ([]byte, error) {
	rtpPacket, err := ParsePacket(pkt[:size])
	if err != nil {
		return nil, err
	}
	hdrLen := rtpPacket.Header.MarshalSize()
	for i := hdrLen; i < size; i += 188 {
		tsPkt := ts.Packet(pkt[i:])
		if tsPkt.IsNull() {
			if log.IsTrace() {
				log.Trace("recovering padding", "bts", strings.TrimRight(hex.Dump(pkt[i:i+16]), "\n"))
			}
			// insert 184 bytes of padding (188 - ts header size)
			if size+184 > len(pkt) {
				return nil, errors.NoTrace("RecoverTsPadding", errors.K.Invalid,
					"reason", "byte slice too small to hold padding",
					"off", i,
					"expected", size+184,
					"actual", len(pkt))
			}
			copy(pkt[i+188:], pkt[i+4:size]) // make room for 184 bytes of padding after the ts header
			copy(pkt[i+4:], padding)         // overwrite freed space with padding
			size += 184                      // adjust RTP packet size
		}
	}
	return pkt[:size], nil
}

// rtpHeader contains minimal RTP header info needed for pacer timing.
// This lightweight struct avoids allocating a full rtp.Packet when only
// sequence number and timestamp are needed.
type rtpHeader struct {
	sequenceNumber uint16
	timestamp      uint32
}

// parseRtpHeaderMinimal extracts only sequence number and timestamp from RTP packet
// without allocating a full rtp.Packet struct. Returns error if packet is invalid.
// Faster than ParsePacket() and allocates nothing on the heap.
func parseRtpHeaderMinimal(packet []byte) (rtpHeader, error) {
	// RTP header minimum size is 12 bytes
	if len(packet) < 12 {
		return rtpHeader{}, errors.NoTrace("parseRtpHeaderMinimal", errors.K.Invalid,
			"reason", "packet too short", "len", len(packet))
	}

	// Check RTP version (2 bits, should be 2)
	version := packet[0] >> 6
	if version != 2 {
		return rtpHeader{}, errors.NoTrace("parseRtpHeaderMinimal", errors.K.Invalid,
			"reason", "invalid RTP version", "version", version)
	}

	// Sequence number: bytes 2-3 (big-endian)
	seq := uint16(packet[2])<<8 | uint16(packet[3])

	// Timestamp: bytes 4-7 (big-endian)
	ts := uint32(packet[4])<<24 | uint32(packet[5])<<16 |
		uint32(packet[6])<<8 | uint32(packet[7])

	return rtpHeader{
		sequenceNumber: seq,
		timestamp:      ts,
	}, nil
}
