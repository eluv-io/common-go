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
