package atomicutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAtomicChannel(t *testing.T) {
	ab := Channel[chanMsg]{}
	require.Nil(t, ab.Get())

	ch := make(chan chanMsg, 1024)
	ab.Set(ch)
	pch := ab.Get()
	require.Equal(t, ch, pch)
	require.Equal(t, 1024, cap(pch))

	ab.SetNX(make(chan chanMsg, 2048))
	require.Equal(t, ch, ab.Get())

	ab.Set(nil)
	require.Nil(t, ab.Get())

	ab.SetNX(make(chan chanMsg, 2048))
	require.Equal(t, 2048, cap(ab.Get()))
}

type chanMsg struct {
	data []byte
	err  error
}
