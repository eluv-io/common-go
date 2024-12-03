package ctxutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelAsContext(t *testing.T) {
	c := make(chan struct{})
	ctx := ChannelAsContext(c)
	require.NoError(t, ctx.Err())
	require.False(t, channelClosed(ctx.Done()))
	require.Nil(t, ctx.Value("any"))
	_, ok := ctx.Deadline()
	require.False(t, ok)

	close(c)
	require.Equal(t, ChannelClosed, ctx.Err())
	require.True(t, channelClosed(ctx.Done()))
}

func channelClosed(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}
