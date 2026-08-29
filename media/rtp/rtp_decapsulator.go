package rtp

import (
	"github.com/eluv-io/common-go/media/pktpool"
)

// NewRtpDecapsulator creates a new decapsulator for RTP payloads.
func NewRtpDecapsulator() *Decapsulator {
	return &Decapsulator{}
}

// Decapsulator is a srtpub.Transformer implementation that decapsulates RTP payloads.
type Decapsulator struct{}

func (r *Decapsulator) Transform(bts []byte) ([]byte, error) {
	return StripHeader(bts)
}

func (r *Decapsulator) TransformPacket(pkt *pktpool.Packet) ([]byte, error) {
	p, err := pkt.Rtp()
	if err != nil {
		return nil, err
	}
	return p.Payload, nil
}
