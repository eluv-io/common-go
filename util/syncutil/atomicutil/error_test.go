package atomicutil

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAtomicError(t *testing.T) {
	ae := &Error{}
	require.Nil(t, ae.Get())

	err := io.EOF
	ae.Set(err)
	require.Equal(t, err, ae.Get())

	err2 := io.ErrUnexpectedEOF
	ae.SetNX(err2)
	require.Equal(t, err, ae.Get())

	ae.Set(nil)
	require.Nil(t, ae.Get())

	ae.SetNX(err2)
	require.Equal(t, err2, ae.Get())
}
