package pktpool_test

import (
	"encoding/binary"
	"testing"

	"github.com/eluv-io/common-go/media/pktpool"
)

// Note: these benchmarks use an inline `if err != nil { b.Fatal(err) }` check rather than
// require.NoError(b, err). require runs on every iteration inside the timed loop and its overhead
// (b.Helper, error formatting machinery) would dominate and distort the per-op timings of these
// nanosecond-scale operations. The inline check compiles down to a cheap nil comparison.

// baseline results:
//
// cpu: Apple M4 Max
// BenchmarkPool_BorrowRelease
// BenchmarkPool_BorrowRelease-14    	44027460	        27.20 ns/op	48815.54 MB/s	       0 B/op	       0 allocs/op
// BenchmarkPacket_From
// BenchmarkPacket_From-14           	81131326	        14.48 ns/op	91696.09 MB/s	       0 B/op	       0 allocs/op
// BenchmarkPacket_DecodeRtp
// BenchmarkPacket_DecodeRtp-14      	41725766	        29.27 ns/op	45366.86 MB/s	       0 B/op	       0 allocs/op
// BenchmarkPacket_DecodeTs
// BenchmarkPacket_DecodeTs-14       	48265540	        24.50 ns/op	53717.47 MB/s	       0 B/op	       0 allocs/op
// BenchmarkPacket_DecodeChain
// BenchmarkPacket_DecodeChain-14    	24998241	        47.55 ns/op	27993.96 MB/s	       0 B/op	       0 allocs/op
// PASS

const benchTsPackets = 7 // 7 * 188 = 1316 bytes, a typical RTP-over-MPEG-TS payload

// BenchmarkPool_BorrowRelease measures the steady-state cost of a borrow/load/release cycle. After warmup the pool
// hands back the same packet, so this should report zero allocations per op.
func BenchmarkPool_BorrowRelease(b *testing.B) {
	raw := rtpBytes(b, 1, 0, tsPayload(benchTsPackets))
	pool := pktpool.NewPacketPool(0, 2048)

	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		pkt := pool.Borrow()
		_ = pkt.T.From(raw)
		pkt.Release()
	}
}

// BenchmarkPacket_From measures the cost of copying input bytes into a packet's buffer (the one unavoidable copy that
// gives the pool ownership of the data).
func BenchmarkPacket_From(b *testing.B) {
	raw := rtpBytes(b, 1, 0, tsPayload(benchTsPackets))
	pool := pktpool.NewPacketPool(0, 2048)
	pkt := pool.Borrow()
	defer pkt.Release()

	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		_ = pkt.T.From(raw)
	}
}

// BenchmarkPacket_DecodeRtp measures lazy RTP decoding only (no pool churn): a single borrowed packet is reloaded via
// From each iteration (which resets decode state) and then decoded.
func BenchmarkPacket_DecodeRtp(b *testing.B) {
	raw := rtpBytes(b, 1, 0, tsPayload(benchTsPackets))
	pool := pktpool.NewPacketPool(0, 2048)
	pkt := pool.Borrow()
	defer pkt.Release()

	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		_ = pkt.T.From(raw)
		if _, err := pkt.T.Rtp(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPacket_DecodeTs measures lazy MPEG-TS decoding only. The TS packets alias the buffer (zero-copy), so this
// should report zero allocations per op once the packet slice is grown.
func BenchmarkPacket_DecodeTs(b *testing.B) {
	ts := tsPayload(benchTsPackets)
	pool := pktpool.NewPacketPool(0, 2048)
	pkt := pool.Borrow()
	defer pkt.Release()

	b.ReportAllocs()
	b.SetBytes(int64(len(ts)))
	for b.Loop() {
		_ = pkt.T.From(ts)
		if _, err := pkt.T.Ts(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPacket_DecodeChain measures the full lazy-decode chain TLV -> RTP -> MPEG-TS on a single reused packet,
// approximating a consumer that walks every protocol layer.
func BenchmarkPacket_DecodeChain(b *testing.B) {
	raw := tlvWrap(0x01, rtpBytes(b, 1, 0, tsPayload(benchTsPackets)))
	pool := pktpool.NewPacketPool(0, 2048)
	pkt := pool.Borrow()
	defer pkt.Release()

	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	for b.Loop() {
		_ = pkt.T.From(raw)
		if _, err := pkt.T.Tlv(); err != nil {
			b.Fatal(err)
		}
		if _, err := pkt.T.Rtp(); err != nil {
			b.Fatal(err)
		}
		if _, err := pkt.T.Ts(); err != nil {
			b.Fatal(err)
		}
	}
}

// tlvWrap prepends a 3-byte TLV header (type + big-endian length) to payload.
func tlvWrap(typ byte, payload []byte) []byte {
	out := make([]byte, 3+len(payload))
	out[0] = typ
	binary.BigEndian.PutUint16(out[1:3], uint16(len(payload)))
	copy(out[3:], payload)
	return out
}
