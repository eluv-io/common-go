package rtp

// NewRtpDecapsulator creates a new decapsulator for RTP payloads.
func NewRtpDecapsulator() *Decapsulator {
	return &Decapsulator{}
}

// Decapsulator is a srtpub.Transformer implementation that decapsulates RTP payloads.
type Decapsulator struct{}

func (r *Decapsulator) Transform(bts []byte) ([]byte, error) {
	return StripHeader(bts)
}
