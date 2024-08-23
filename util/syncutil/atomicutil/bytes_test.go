package atomicutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAtomicBytes(t *testing.T) {
	ab := Bytes{}
	require.Nil(t, ab.Get())

	buf := []byte("bla")
	ab.Set(buf)
	require.Equal(t, buf, ab.Get())

	buf2 := []byte("argh")
	ab.SetNX(buf2)
	require.Equal(t, buf, ab.Get())

	ab.Set(nil)
	require.Nil(t, ab.Get())

	ab.SetNX(buf2)
	require.Equal(t, buf2, ab.Get())
}
