package atomicutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAtomicBytes(t *testing.T) {
	as := Bytes{}
	require.Nil(t, as.Get())
	as.Set([]byte("bla"))
	require.Equal(t, "bla", string(as.Get()))
	as.Set(nil)
	require.Nil(t, as.Get())
}
