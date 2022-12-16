package bitutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAxor(t *testing.T) {
	a := []byte{4, 8, 12, 3, 7, 4}
	b := []byte{8, 12, 4, 7, 4, 3}
	e := []byte{12, 4, 8, 4, 3, 7}
	c, err := Axor(a, b)
	require.NoError(t, err)
	require.Equal(t, e, c)

	// since xor we also have
	aa, err := Axor(c, b)
	require.NoError(t, err)
	bb, err := Axor(a, c)
	require.NoError(t, err)

	require.Equal(t, a, aa)
	require.Equal(t, b, bb)
}

func TestShiftLeft(t *testing.T) {
	a := []byte{36, 72, 12, 3, 7, 4}
	dst := make([]byte, len(a))
	ShiftLeft(dst, a, 2)

	//36, 72, 12, 3, 7, 4
	// 00100100 01001000 00001100 00000011 00000111 00000100
	// 10010001 00100000 00110000 00001100 00011100 00010000
	//145, 32, 48, 28, 16

	e := []byte{145, 32, 48, 12, 28, 16}
	require.Equal(t, e, dst)

	// shift in place
	a = []byte{36, 72, 12, 3, 7, 4}
	ShiftLeft(a, a, 2)
	require.Equal(t, e, a)
}

func TestShiftRight(t *testing.T) {
	a := []byte{145, 32, 48, 12, 28, 16}
	dst := make([]byte, len(a))
	ShiftRight(dst, a, 2)

	e := []byte{36, 72, 12, 3, 7, 4}
	require.Equal(t, e, dst)

	// shift in place
	a = []byte{145, 32, 48, 12, 28, 16}
	ShiftRight(a, a, 2)
	require.Equal(t, e, a)
}

func TestDecodeString(t *testing.T) {
	// Hex: 0cdf386b4ad85ff1a04d83e25f414acb772611cac8a9fc34f11c49274e3b56be
	b, err := DecodeString("0000110011011111001110000110101101001010110110000101111111110001101000000100110110000011111000100101111101000001010010101100101101110111001001100001000111001010110010001010100111111100001101001111000100011100010010010010011101001110001110110101011010111110")
	require.NoError(t, err)
	require.Equal(t, []byte{12, 223, 56, 107, 74, 216, 95, 241, 160, 77, 131, 226, 95, 65, 74, 203, 119, 38, 17, 202, 200, 169, 252, 52, 241, 28, 73, 39, 78, 59, 86, 190}, b)

	// "0b" prefix, Hex: 0ec66e08882614c843bc6930383c1d22fbd809e0
	b, err = DecodeString("0b0000111011000110011011100000100010001000001001100001010011001000010000111011110001101001001100000011100000111100000111010010001011111011110110000000100111100000")
	require.NoError(t, err)
	require.Equal(t, []byte{14, 198, 110, 8, 136, 38, 20, 200, 67, 188, 105, 48, 56, 60, 29, 34, 251, 216, 9, 224}, b)

	// Invalid binary string length
	b, err = DecodeString("010101010101")
	require.Error(t, err)

	// Invalid binary string
	b, err = DecodeString("helloworld")
	require.Error(t, err)
}

func TestEncodeToString(t *testing.T) {
	// Hex: 0cdf386b4ad85ff1a04d83e25f414acb772611cac8a9fc34f11c49274e3b56be
	s := EncodeToString([]byte{12, 223, 56, 107, 74, 216, 95, 241, 160, 77, 131, 226, 95, 65, 74, 203, 119, 38, 17, 202, 200, 169, 252, 52, 241, 28, 73, 39, 78, 59, 86, 190})
	require.Equal(t, "0000110011011111001110000110101101001010110110000101111111110001101000000100110110000011111000100101111101000001010010101100101101110111001001100001000111001010110010001010100111111100001101001111000100011100010010010010011101001110001110110101011010111110", s)

	// Hex: 0ec66e08882614c843bc6930383c1d22fbd809e0
	s = EncodeToString([]byte{14, 198, 110, 8, 136, 38, 20, 200, 67, 188, 105, 48, 56, 60, 29, 34, 251, 216, 9, 224})
	require.Equal(t, "0000111011000110011011100000100010001000001001100001010011001000010000111011110001101001001100000011100000111100000111010010001011111011110110000000100111100000", s)
}
