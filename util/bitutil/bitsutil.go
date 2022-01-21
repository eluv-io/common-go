package bitutil

import "github.com/eluv-io/errors-go"

// axor (array xor) calculates the xor across two byte slices.
// Returns an error if the slices are of different lengths.
func Axor(a1, a2 []byte) ([]byte, error) {
	if len(a1) != len(a2) {
		err := errors.E("axor", errors.K.Invalid,
			"reason", "array lengths are different",
			"a1", a1,
			"a2", a2)
		return nil, err
	}
	res := make([]byte, len(a1))
	for i, v := range a1 {
		res[i] = v ^ a2[i]
	}
	return res, nil
}

/*
PENDING(GIL): shift functions:
  - implement rotation
*/

// ShiftLeft performs a left bit shift operation on the provided bytes.
// bits must be between 0 and 7 bits and equal src and dst lengths.
func ShiftLeft(dst, src []byte, bits uint8) {
	if len(src) == 0 {
		return
	}
	bits &= 0x7
	trunc := 8 - bits
	last := len(src) - 1
	for i := 0; i < last; i++ {
		dst[i] = src[i]<<bits | src[i+1]>>trunc
	}
	dst[last] = src[last] << bits
}

func ShiftL(src []byte, bits uint8) []byte {
	dst := make([]byte, len(src))
	ShiftLeft(dst, src, bits)
	return dst
}

// ShiftRight performs a right bit shift operation on the provided bytes.
// bits must be between 0 and 7 bits and equal src and dst lengths.
func ShiftRight(dst, src []byte, bits uint8) {
	if len(src) == 0 {
		return
	}
	bits &= 0x7
	trunc := 8 - bits
	for i := len(src) - 1; i > 0; i-- {
		dst[i] = src[i]>>bits | src[i-1]<<trunc
	}
	dst[0] = src[0] >> bits
}

func ShiftR(src []byte, bits uint8) []byte {
	dst := make([]byte, len(src))
	ShiftRight(dst, src, bits)
	return dst
}
