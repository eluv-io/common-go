package bitutil

import (
	"strconv"
	"strings"

	"github.com/eluv-io/errors-go"
)

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

// DecodeString decodes the given binary string to the represented bytes.
// Expects a string of 0s and 1s, with length divisible by 8, with or without the "0b" prefix.
func DecodeString(s string) ([]byte, error) {
	if strings.HasPrefix(s, "0b") {
		s = s[2:]
	}

	if len(s)%8 != 0 {
		return nil, errors.E("decode bit string", errors.K.Invalid,
			"reason", "binary string length not divisible by 8",
			"string", s)
	}

	b := make([]byte, 0, len(s)/8)
	for i := 0; i < len(s); i += 8 {
		n, err := strconv.ParseUint(s[i:i+8], 2, 8)
		if err != nil {
			return nil, errors.E("decode bit string", errors.K.Invalid, err,
				"string", s)
		}
		b = append(b, byte(n))
	}

	return b, nil
}

// EncodeToString encodes the given bytes into a binary string.
// Does not add the "0b" prefix.
func EncodeToString(b []byte) string {
	s := ""
	for _, n := range b {
		x := strconv.FormatInt(int64(n), 2)
		s += strings.Repeat("0", 8-len(x)) + x
	}
	return s
}
