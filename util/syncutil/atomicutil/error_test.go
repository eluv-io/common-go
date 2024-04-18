package atomicutil

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/errors-go"
)

func TestAtomicError(t *testing.T) {
	ae := &Error{}

	require.Nil(t, ae.Get())
	err := io.EOF
	ae.Set(err)
	require.Equal(t, io.EOF, ae.Get())

	err = errors.NoTrace("bla")
	ae.Set(err)
	require.Equal(t, err, ae.Get())

	ae.Set(nil)
	require.Nil(t, ae.Get())
}
