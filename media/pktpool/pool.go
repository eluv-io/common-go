package pktpool

import (
	"github.com/eluv-io/common-go/collections/pool"
)

// Resource is a reference-counted Packet borrowed from a packet pool.
type Resource = *pool.Resource[*Packet]

// Pool is a pool of Packets. Borrow a Packet with Pool.Borrow.
type Pool = pool.Pool[*Packet]

// NewPacketPool creates a pool of Packets. Each packet has wrapCap bytes of headroom before its payload area and cap
// bytes of payload capacity; wrapCap is used by Packet.WrapTlv to prepend a TLV header without copying the payload.
func NewPacketPool(wrapCap, cap int) *pool.Pool[*Packet] {
	return pool.New(factory{
		wrapCap: wrapCap,
		cap:     cap,
	})
}

type factory struct {
	wrapCap, cap int
}

func (f factory) New() *Packet {
	return NewPacket(f.wrapCap, f.cap)
}

func (f factory) Init(p *Packet) {
	p.init()
}

func (f factory) Reset(p *Packet) {
	p.reset()
}
