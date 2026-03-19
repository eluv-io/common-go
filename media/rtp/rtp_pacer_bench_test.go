package rtp_test

import (
	"sync"
	"testing"

	rtp2 "github.com/eluv-io/common-go/media/rtp"
)

// BenchmarkRtpPacer_PushPop measures the steady-state cost of Push+Pop through the channel buffer.
// With Delay=0 the consumer never sleeps, so no timer allocations occur on the consumer side.
//
// Unlike DisruptorPacer, RtpPacer allocates on every Push: ParsePacket returns a heap-allocated
// *rtp.Packet, and newPacerPacket allocates the slot struct and copies the payload.
func BenchmarkRtpPacer_PushPop(b *testing.B) {
	pacer := rtp2.NewRtpPacer() // Delay=0: consumer never sleeps

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			if _, err := pacer.Pop(); err != nil {
				return
			}
		}
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
