package media

import (
	"context"
	"time"

	"github.com/eluv-io/common-go/media/pktpool"
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
		packetCh:   make(chan *pktpool.Packet, chanSize),
		packetPool: pktpool.NewPacketPool(2048), // PENDING(SS) use proper packet size
	}
	p.ctx, p.cancel = context.WithCancelCause(context.Background())
	return p
}

type NoopAsyncPacer struct {
	packetCh         chan *pktpool.Packet
	packetPool       *pktpool.PacketPool
	lastPoppedPacket *pktpool.Packet         // track last packet to release it on next Pop()
	ctx              context.Context         // context for canceling the pacer
	cancel           context.CancelCauseFunc // to cancel the pacer
}

func (n *NoopAsyncPacer) Push(bts []byte) error {
	// Use pre-allocated pool
	pkt := n.packetPool.GetPacket()
	pkt.Data = pkt.Data[:len(bts)]
	copy(pkt.Data, bts)

	select {
	case n.packetCh <- pkt:
		return nil
	case <-n.ctx.Done():
		// Release the packet since we couldn't send it
		pkt.Release()
		return n.ctx.Err()
	}
}

func (n *NoopAsyncPacer) Pop() (pkt []byte, err error) {
	// Release the previous packet (if any) since the caller is done with it
	if n.lastPoppedPacket != nil {
		n.lastPoppedPacket.Release()
		n.lastPoppedPacket = nil
	}

	select {
	case pooledPkt := <-n.packetCh:
		// Store reference to this packet so we can release it on the next Pop() call
		n.lastPoppedPacket = pooledPkt
		return pooledPkt.Data, nil
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

	// Release the last popped packet if any
	if n.lastPoppedPacket != nil {
		n.lastPoppedPacket.Release()
		n.lastPoppedPacket = nil
	}

	// Drain and release any remaining packets in the channel
	for {
		select {
		case pkt := <-n.packetCh:
			pkt.Release()
		default:
			return
		}
	}
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
