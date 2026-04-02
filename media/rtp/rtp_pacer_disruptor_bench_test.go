package rtp_test

import (
	"sync"
	"testing"

	pionrtp "github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/utc-go"
)

// BenchmarkDisruptorPacer_PushRun measures the steady-state cost of Push through the ring buffer. With Delay=0 the
// consumer never sleeps (wait ≤ 0 ≤ MinSleepThreshold), so no time.After allocations occur. A warmup pass
// pre-populates every ring buffer slot's pkt slice so the hot loop never calls make([]byte, ...).
//
// Expected: 0 allocs/op.
//
// Baseline - without statistics:
// goos: darwin
// goarch: arm64
// pkg: github.com/eluv-io/common-go/media/rtp
// cpu: Apple M4 Max
// BenchmarkDisruptorPacer_PushRun
// BenchmarkDisruptorPacer_PushRun-14    	 9020377	       123.4 ns/op	       0 B/op	       0 allocs/op
// PASS
//
// Baseline - with statistics:
// goos: darwin
// goarch: arm64
// pkg: github.com/eluv-io/common-go/media/rtp
// cpu: Apple M4 Max
// BenchmarkDisruptorPacer_PushRun
// BenchmarkDisruptorPacer_PushRun-14    	 5450133	       229.0 ns/op	       1 B/op	       0 allocs/op
func BenchmarkDisruptorPacer_PushRun(b *testing.B) {
	pacer := newTestDisruptorPacer(b, 0, 0) // Delay=0: consumer never sleeps

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = pacer.Run(func([]byte, utc.UTC) error { return nil })
	}()

	raw := makeBenchPacket(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = pacer.Push(raw)
	}

	pacer.Shutdown()
	wg.Wait()

	b.StopTimer()
}

// makeBenchPacket marshals a standard RTP packet with a 1316-byte payload (7 MPEG-TS packets).
func makeBenchPacket(b *testing.B) []byte {
	b.Helper()
	pkt := &pionrtp.Packet{
		Header:  pionrtp.Header{SequenceNumber: 0, Timestamp: 0},
		Payload: make([]byte, 7*188),
	}
	raw, err := pkt.Marshal()
	require.NoError(b, err)
	return raw
}
