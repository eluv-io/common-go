package byteutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRingBuffer(t *testing.T) {
	w10 := []byte("1234567890")
	r17 := make([]byte, 17)
	var written, read int

	r := NewRingBuffer(17)

	for i := 0; i < 5; i++ {
		written = r.Write(w10)
		require.Equal(t, 10, written)

		written = r.Write(w10)
		require.Equal(t, 7, written)

		read = r.Read(r17)
		require.Equal(t, 17, read)
		require.Equal(t, []byte("12345678901234567"), r17)

		written = r.Write(w10[7:])
		require.Equal(t, 3, written)

		written = r.Write(w10)
		require.Equal(t, 10, written)

		written = r.Write(w10)
		require.Equal(t, 4, written)

		read = r.Read(r17)
		require.Equal(t, 17, read)
		require.Equal(t, []byte("89012345678901234"), r17)
	}
}

func TestRingBuffer_Unread(t *testing.T) {
	w10 := []byte("1234567890")
	r7 := make([]byte, 7)

	r := NewRingBuffer(17)

	require.NoError(t, r.Unread(0))
	require.Error(t, r.Unread(1))
	require.Error(t, r.Unread(5))

	r.Write(w10)
	require.NoError(t, r.Unread(0))
	require.Error(t, r.Unread(1))
	require.Error(t, r.Unread(5))

	require.Equal(t, 7, r.Read(r7))
	require.Equal(t, []byte("1234567"), r7)
	require.Error(t, r.Unread(10))

	require.NoError(t, r.Unread(3))
	require.Equal(t, 6, r.Read(r7))
	require.Equal(t, []byte("567890"), r7[:6])

	require.Equal(t, 10, r.Write(w10))
	for i := 0; i < 7; i++ {
		require.NoError(t, r.Unread(1), "i=%d", i)
	}
	require.Error(t, r.Unread(1))
}
