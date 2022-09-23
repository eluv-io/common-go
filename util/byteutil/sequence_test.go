package byteutil

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSequence(t *testing.T) {
	seq := &Sequence{}

	for i := 0; i < 256; i++ {
		require.Equal(t, []byte{byte(i)}, seq.Next(), i)
	}
	for i := 1; i < 256; i++ {
		for j := 0; j < 256; j++ {
			require.Equal(t, []byte{byte(i), byte(j)}, seq.Next(), j)
		}
	}

	seq = NewSequence(math.MaxUint32)
	require.Equal(t, []byte{255, 255, 255, 255}, seq.Next())
	require.Equal(t, []byte{1, 0, 0, 0, 0}, seq.Next())
	require.Equal(t, []byte{1, 0, 0, 0, 1}, seq.Next())

	seq = NewSequence(0)
	require.Equal(t, []byte{0}, seq.Next())

	seq = NewSequence(1)
	require.Equal(t, []byte{1}, seq.Next())

	seq = NewSequence(10)
	require.Equal(t, []byte{10}, seq.Next())
}
