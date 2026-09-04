package pktpool_test

import (
	"fmt"
	"sync"

	"github.com/pion/rtp"

	"github.com/eluv-io/common-go/media/pktpool"
)

// Example shows the basic lifecycle: borrow a pooled packet, load received bytes into it, lazily decode the protocol
// layers (outermost first), and release it back to the pool when done.
func Example() {
	// A pool hands out fixed-capacity, reference-counted packets backed by pooled buffers, avoiding per-packet
	// allocations on the hot path. wrapCap=0 (no head room for WrapTlv), cap=1500 bytes of payload capacity.
	pool := pktpool.NewPacketPool(0, 1500)

	// Simulate data received from the network: an RTP packet carrying two MPEG-TS packets.
	ts := make([]byte, 2*188)
	ts[0], ts[188] = 0x47, 0x47 // MPEG-TS sync bytes
	raw, _ := (&rtp.Packet{
		Header:  rtp.Header{Version: 2, SequenceNumber: 100, Timestamp: 9000},
		Payload: ts,
	}).Marshal()

	// Borrow a packet and copy the received bytes into its pooled buffer (the one unavoidable copy).
	pkt := pool.Borrow()
	if err := pkt.T.From(raw); err != nil {
		panic(err)
	}

	// Lazily decode the layers present, outermost first. This data is raw RTP carrying MPEG-TS, so decode RTP, then
	// the TS packets it carries. Each layer aliases the buffer (zero-copy) and is parsed at most once.
	rtpLayer, err := pkt.T.Rtp()
	if err != nil {
		panic(err)
	}
	tsLayer, err := pkt.T.Ts()
	if err != nil {
		panic(err)
	}

	fmt.Println("rtp sequence:", rtpLayer.Packet().SequenceNumber)
	fmt.Println("ts packets:", len(tsLayer.Packets()))

	// Release the packet's single reference; it returns to the pool for reuse.
	pkt.Release()

	// Output:
	// rtp sequence: 100
	// ts packets: 2
}

// Example_sharing shows how to share one packet between multiple concurrent consumers using reference counting. The
// owner decodes the needed layer first, then hands the packet to the consumers; the packet returns to the pool only
// once every reference has been released.
func Example_sharing() {
	pool := pktpool.NewPacketPool(0, 1500)

	raw, _ := (&rtp.Packet{
		Header:  rtp.Header{Version: 2, SequenceNumber: 7},
		Payload: []byte("media payload"),
	}).Marshal()

	pkt := pool.Borrow()
	if err := pkt.T.From(raw); err != nil {
		panic(err)
	}

	// Decode the layer once, here on the owning goroutine, BEFORE sharing. Consumers then re-read the cached layer
	// concurrently (safe); they must not trigger decoding themselves (see the concurrency notes on Packet).
	if _, err := pkt.T.Rtp(); err != nil {
		panic(err)
	}

	const consumers = 2
	results := make(chan int, consumers)
	var wg sync.WaitGroup
	for range consumers {
		pkt.Reference() // one extra reference per consumer
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer pkt.Release() // each consumer releases its reference when finished
			r, _ := pkt.T.Rtp() // returns the cached layer, no decoding
			results <- len(r.Payload)
		}()
	}

	pkt.Release() // drop the owner's original reference; the last Release returns the packet to the pool
	wg.Wait()
	close(results)

	total := 0
	for n := range results {
		total += n
	}
	fmt.Println("consumers:", consumers)
	fmt.Println("total payload bytes read:", total)

	// Output:
	// consumers: 2
	// total payload bytes read: 26
}
