package tlv

import "github.com/eluv-io/common-go/media/tlv/tlv"

// NewTlvEncapsulator creates a new Encapsulator for TLV.
func NewTlvEncapsulator(typ byte) *Encapsulator {
	return &Encapsulator{typ: typ}
}

// Encapsulator is a srtpub.Transformer implementation that encapsulates payloads in TLV.
type Encapsulator struct {
	typ byte
}

func (e *Encapsulator) Transform(bts []byte) ([]byte, error) {
	res := make([]byte, 3+len(bts))
	copy(res[3:], bts)
	err := tlv.WriteHeader(res[:3], e.typ, uint16(len(bts)))
	if err != nil {
		return nil, err
	}
	return res, nil
}
