package atomicutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAtomicString(t *testing.T) {
	as := String{}
	require.Equal(t, "", as.Get())

	str := "bla"
	as.Set(str)
	require.Equal(t, str, as.Get())

	str2 := "argh"
	as.SetNX(str2)
	require.Equal(t, str, as.Get())

	as.Set("")
	require.Equal(t, "", as.Get())

	as.SetNX(str2)
	require.Equal(t, str2, as.Get())
}
