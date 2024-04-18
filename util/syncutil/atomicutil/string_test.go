package atomicutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAtomicString(t *testing.T) {
	as := String{}
	require.Equal(t, "", as.Get())
	as.Set("bla")
	require.Equal(t, "bla", as.Get())
	as.Set("")
	require.Equal(t, "", as.Get())
}
