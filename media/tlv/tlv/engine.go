package tlv

import "github.com/eluv-io/errors-go"

// ParseHeader parses a TLV header from the given 3-byte array. Returns the type and length.
func ParseHeader(bts [3]byte) (typ byte, len uint16) {
	typ = bts[0]
	len = uint16(bts[1])<<8 | uint16(bts[2])
	return
}

// WriteHeader writes the given type and length to the given bytes slice in TLV header format. Returns an error if
// the buffer is too small.
func WriteHeader(bts []byte, typ byte, length uint16) error {
	if len(bts) < 3 {
		return errors.NoTrace("tlv.WriteHeader", errors.K.Invalid, "reason", "buffer too small", "len", len(bts))
	}
	bts[0] = typ
	bts[1] = byte(length >> 8)
	bts[2] = byte(length)
	return nil
}
