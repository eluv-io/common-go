package media

import (
	"context"
	"time"

	"github.com/eluv-io/common-go/util/ifutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
)

func NewNoopPacketizer() Packetizer {
	return &NoopPacketizer{}
}

type NoopPacketizer struct {
	pkt     []byte
	pktSize int
}

func (n *NoopPacketizer) Write(bts []byte) {
	n.pkt = bts
}

func (n *NoopPacketizer) Next() ([]byte, error) {
	if n.pkt == nil {
		return nil, nil
	}
	pkt := n.pkt
	n.pktSize = len(n.pkt)
	n.pkt = nil
	return pkt, nil
}

func (n *NoopPacketizer) TargetPacketSize() int {
	return n.pktSize
}

// ---------------------------------------------------------------------------------------------------------------------

func NewNoopPacer() Pacer {
	return NoopPacer{}
}

type NoopPacer struct{}

func (n NoopPacer) Wait([]byte)                                         {}
func (n NoopPacer) CalculateWait(now utc.UTC, bts []byte) time.Duration { return 0 }
func (n NoopPacer) SetDelay(time.Duration)                              {}

// ---------------------------------------------------------------------------------------------------------------------

func NewNoopAsyncPacer(chanSize uint) AsyncPacer {
	p := &NoopAsyncPacer{
		packetCh: make(chan []byte, chanSize),
	}
	p.ctx, p.cancel = context.WithCancelCause(context.Background())
	return p
}

type NoopAsyncPacer struct {
	packetCh chan []byte
	ctx      context.Context         // context for canceling the pacer
	cancel   context.CancelCauseFunc // to cancel the pacer
}

func (n *NoopAsyncPacer) Push(bts []byte) error {
	clone := make([]byte, len(bts))
	copy(clone, bts)

	select {
	case n.packetCh <- clone:
		return nil
	case <-n.ctx.Done():
		return n.ctx.Err()
	}
}

func (n *NoopAsyncPacer) Pop() (pkt []byte, err error) {
	select {
	case pkt = <-n.packetCh:
		return pkt, nil
	case <-n.ctx.Done():
		return nil, n.ctx.Err()
	}
}

func (n *NoopAsyncPacer) Shutdown(err ...error) {
	n.cancel(
		ifutil.FirstOrDefault[error](
			err,
			errors.E("noopAsyncPacer.Shutdown", errors.K.Cancelled, "reason", "pacer shutdown"),
		),
	)
}

func (n *NoopAsyncPacer) Wait([]byte)                                         {}
func (n *NoopAsyncPacer) CalculateWait(now utc.UTC, bts []byte) time.Duration { return 0 }
func (n *NoopAsyncPacer) SetDelay(time.Duration)                              {}

// ---------------------------------------------------------------------------------------------------------------------

func NewNoopTransformer() Transformer {
	return NoopTransformer{}
}

type NoopTransformer struct{}

func (n NoopTransformer) Transform(bts []byte) ([]byte, error) {
	return bts, nil
}
