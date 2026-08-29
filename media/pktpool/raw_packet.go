package pktpool

import (
	"github.com/eluv-io/errors-go"
)

// Decoder is the lazy, cached, order-enforced RTP/MPEG-TS layer decoding shared by Packet and RawPacket - the two
// ways to obtain a decoded packet's layers: a pool-managed Packet, or a standalone RawPacket wrapping caller-owned
// memory. See the Packet type doc for the layer-ordering contract, which applies here unchanged.
type Decoder interface {
	Rtp() (*RtpPacket, error)
	Ts() (*TsPacket, error)
}

// RawPacket provides Decoder's lazy RTP/MPEG-TS layer decoding directly over a caller-owned byte slice, with no
// copying, no backing buffer, and no size limit. It is not part of the Pool/Resource machinery - no Init/Reset
// factory hook, no reference counting - so it must not be shared across goroutines or reused concurrently; it is
// meant for a single owner reusing one instance across sequential calls (e.g. one MediaTracker instance). Reset
// re-points the RawPacket at a new slice, discarding previously decoded layers; anything obtained from a previous
// Reset (Payload slices, decoded headers) is invalidated.
type RawPacket struct {
	data     []byte // remaining undecoded data (the lazy-decode cursor)
	lastRank int
	rtp      RtpPacket
	ts       TsPacket
}

// NewRawPacket creates a RawPacket wrapping data. Equivalent to new(RawPacket) followed by Reset(data).
func NewRawPacket(data []byte) *RawPacket {
	return (&RawPacket{}).Reset(data)
}

// Reset re-points the RawPacket at data, discarding any previously decoded layers. data is aliased, not copied.
func (p *RawPacket) Reset(data []byte) *RawPacket {
	p.data = data
	p.lastRank = -1
	p.rtp.reset()
	p.ts.reset()
	return p
}

// decodeInOrder mirrors Packet.decodeInOrder (see its doc for the layer-ordering contract), but advances a plain
// slice cursor instead of a buf/offset pair, since RawPacket has no owning buffer to index into.
func (p *RawPacket) decodeInOrder(rank int, decode func([]byte) (int, int, error)) error {
	if rank <= p.lastRank {
		return errors.NoTrace("pktpool.RawPacket", errors.K.Invalid,
			"reason", "protocol layer decoded out of order",
			"rank", rank,
			"last_rank", p.lastRank)
	}
	head, tail, err := decode(p.data)
	if err != nil {
		return err
	}
	p.data = p.data[head : len(p.data)-tail]
	p.lastRank = rank
	return nil
}

// Rtp returns the RTP packet contained in this RawPacket, parsing it on first access and caching the result.
func (p *RawPacket) Rtp() (*RtpPacket, error) {
	if p.rtp.parsed {
		return &p.rtp, nil
	}
	return &p.rtp, p.decodeInOrder(rankRtp, p.rtp.decode)
}

// Ts returns the MPEG-TS packets contained in the remaining data, parsing them on first access and caching the
// result.
func (p *RawPacket) Ts() (*TsPacket, error) {
	if p.ts.parsed {
		return &p.ts, nil
	}
	return &p.ts, p.decodeInOrder(rankTs, p.ts.decode)
}
