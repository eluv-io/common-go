package ctxutil

import (
	"time"

	"github.com/eluv-io/errors-go"
)

var ChannelClosed = errors.Str("channel closed")

// ChannelAsContext adapts this channel as a context.Context. The returned context's Done() channel is the provided
// channel, so it is closed when the provided channel is closed. The Err() method will return ChannelClosed if the
// channel was closed.
func ChannelAsContext(ch chan struct{}) *ChannelCtx {
	return &ChannelCtx{Channel: ch}
}

type ChannelCtx struct {
	Channel chan struct{}
}

func (s *ChannelCtx) Deadline() (deadline time.Time, ok bool) {
	return
}

func (s *ChannelCtx) Done() <-chan struct{} {
	return s.Channel
}

func (s *ChannelCtx) Err() error {
	select {
	case <-s.Channel:
		// An error must be returned to satisfy Context: it panics when Done
		// and no error returned.
		return ChannelClosed
	default:
		// Continue
	}
	return nil
}

func (s *ChannelCtx) Value(key interface{}) interface{} {
	return nil
}
