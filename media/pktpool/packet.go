package pktpool

import (
	"sync"
	"sync/atomic"

	"github.com/eluv-io/errors-go"
)

// Packet is a wrapper around a byte slice that tracks the number of references to it and releases it back to the packet
// pool when the last reference is dropped.
type Packet struct {
	Data []byte
	data []byte // reference to original slice for resetting to full capacity
	refs atomic.Int32
	pool *sync.Pool
}

// Reference increments the reference count by one or the given number.
func (p *Packet) Reference(count ...uint16) {
	c := int32(1)
	if len(count) > 0 {
		c = int32(count[0])
	}
	p.refs.Add(c)
}

// Release decrements the reference count by one or the given number and returns the packet to the pool if the count
// reaches zero. Panics if the reference count drops below zero.
func (p *Packet) Release(count ...uint16) {
	c := int32(1)
	if len(count) > 0 {
		c = int32(count[0])
	}
	refs := p.refs.Add(-c)
	if refs == 0 {
		p.pool.Put(p)
	} else if refs < 0 {
		// This is not thread-safe (another go-routine might already be retrieving the same packet from the pool and be
		// modifying the ref count). However, it's mainly used to detect programming errors (duplicate releases), so I
		// think this is acceptable.
		panic(errors.E("PacketPool.Release", errors.K.Invalid, "reason", "negative reference count!", "count", refs))
	}
}
