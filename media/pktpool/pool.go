package pktpool

import (
	"sync"
)

// NewPacketPool creates a new PacketPool with the given packet size.
func NewPacketPool(packetSize ...int) *PacketPool {
	var packetPool PacketPool
	packetPool.pool.New = func() interface{} {
		pktSize := 2048
		if len(packetSize) > 0 && packetSize[0] > 0 {
			pktSize = packetSize[0]
		}
		return &Packet{
			data: make([]byte, pktSize), // default larger than max MTU/SRT payload (1500)
			pool: &packetPool.pool,
		}
	}
	return &packetPool
}

// PacketPool is a pool for fixed-capacity byte slices that can be shared between multiple processes. To simplify usage,
// the byte slices are wrapped in a Packet struct that tracks the number of references to it. When the last reference is
// released, the packet is returned to the pool.
//
// Initially, a packet's ref count is 1 when it is retrieved from the pool with pool.GetPacket(). If you want to share
// the packet with another process, increase the ref count with p.Reference(count) before handing the packet over. The
// other process will then call p.Release() when finished with the packet.
type PacketPool struct {
	pool sync.Pool
}

// GetPacket returns a Packet from the packet pool. Its reference count is set to 1. The Data field byte slice is
// guaranteed to be of the same capacity as the pool's initial packet size, but it may contain data from a previous use
// (it is not zeroed).
func (p *PacketPool) GetPacket() *Packet {
	pkt := p.pool.Get().(*Packet)
	pkt.Data = pkt.data // reset to full capacity
	pkt.refs.Store(1)   // initialize ref count
	return pkt
}
