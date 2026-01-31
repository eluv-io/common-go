package tlv

import (
	"github.com/eluv-io/common-go/media/tlv/tlv"
	"github.com/eluv-io/errors-go"
)

// NewTlvDecapsulator creates a new decapsulator for TLV payloads.
func NewTlvDecapsulator() *Decapsulator {
	return &Decapsulator{}
}

// Decapsulator is a srtpub.Transformer implementation that decapsulates TLV payloads.
type Decapsulator struct{}

func (r *Decapsulator) Transform(bts []byte) ([]byte, error) {
	if len(bts) < 3 {
		return nil, errors.NoTrace("TlvDecapsulator.Transform", errors.K.Invalid, "reason", "header too short")
	}
	_, size := tlv.ParseHeader([3]byte(bts[:3]))
	if 3+int(size) > len(bts) {
		return nil, errors.NoTrace("TlvDecapsulator.Transform", errors.K.Invalid, "reason", "payload too short")
	}
	return bts[3:], nil
}
