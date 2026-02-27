package rtp

import (
	"github.com/pion/rtp"

	"github.com/eluv-io/common-go/util/byteutil"
	"github.com/eluv-io/errors-go"
)

const (
	minRtpHeaderLen = 12  // minimum size of an RTP header
	maxRtpHeaderLen = 200 // arbitrary, but larger than the minimum of 12 bytes...
	tsPacketLen     = 188
)

type options struct {
	tsPacketCount int
}

type Option func(*options)

func OptTsPacketCount(count int) Option {
	return func(o *options) {
		o.tsPacketCount = count
	}
}

// NewRtpPacketizer creates a new packetizer for MPEG-TS over RTP. It can be used to extract RTP packets from a
// contiguous stream of bytes that was created by concatenating RTP packets. The RTP packets are expected to carry a
// potentially variable number of MPEG-TS packets.
func NewRtpPacketizer(opts ...Option) *Packetizer {
	var o options
	OptTsPacketCount(7)(&o)
	for _, opt := range opts {
		opt(&o)
	}
	if o.tsPacketCount < 1 {
		o.tsPacketCount = 1
	}

	maxPacketSize := maxRtpHeaderLen + o.tsPacketCount*tsPacketLen

	return &Packetizer{
		exptectedPayloadLen: o.tsPacketCount * tsPacketLen,
		buf:                 byteutil.NewRingBuffer(maxPacketSize),
		pkt:                 make([]byte, maxPacketSize),
		options:             o,
	}
}

// Packetizer is a packetizer for concatenated RTP packets.
type Packetizer struct {
	options             options
	exptectedPayloadLen int

	buf       *byteutil.RingBuffer // internal ring buffer for packetization
	pkt       []byte               // the next packet to be returned
	remaining []byte               // the remaining bytes from the last Write call that did not fit into the ring buffer

	// parse state

	alreadyRead int  // number of bytes already read from the ring buffer into pkt
	haveHeader  bool // true rtp header has been parsed
	headerLen   int  // the size of the parsed header
	totalLen    int  // the size of the full rtp packet including header (once the header has been parsed)
	consumed    int  // number of bytes consumed by the last call to Next()
}

func (p *Packetizer) Write(bts []byte) {
	written := p.buf.Write(bts)
	p.remaining = bts[written:]
}

func (p *Packetizer) Next() ([]byte, error) {
	p.consumed = 0
	for {
		p.Write(p.remaining) // no-op if remaining is empty...
		if p.buf.Len() == 0 {
			return nil, nil
		}
		p.alreadyRead += p.buf.Read(p.pkt[p.alreadyRead:])

		if !p.haveHeader {
			if p.alreadyRead < minRtpHeaderLen {
				continue
			}

			hdr := rtp.Header{}
			hdrLen, err := hdr.Unmarshal(p.pkt[:p.alreadyRead])
			if err != nil {
				// not enough bytes to parse header
				continue
			}

			if hdrLen > maxRtpHeaderLen {
				return nil, errors.E("rtpPacketizer.Next", errors.K.Invalid,
					"reason", "header len exceeded",
					"len", hdrLen)
			}
			if hdr.Padding {
				return nil, errors.E("rtpPacketizer.Next", errors.K.Invalid, "reason", "padding not supported")
			}

			p.haveHeader = true
			p.headerLen = hdrLen
		}

		p.totalLen = p.headerLen + p.exptectedPayloadLen
		if p.alreadyRead < p.totalLen {
			continue
		}

		err := p.buf.Unread(p.alreadyRead - p.totalLen)
		if err != nil {
			return nil, errors.E("rtpPacketizer.Next", errors.K.Internal, err,
				"reason", "failed to unread bytes",
				"count", p.alreadyRead-p.totalLen)
		}

		p.consumed += p.totalLen

		// reset state
		p.haveHeader = false
		p.alreadyRead = 0

		return p.pkt[:p.totalLen], nil
	}
}

func (p *Packetizer) TargetPacketSize() int {
	return p.totalLen
}

// Consumed returns the number of bytes consumed to produce the last packet returned by Next(). Only call after Next()
// returns a non-nil packet.
func (p *Packetizer) Consumed() int {
	return p.consumed
}
