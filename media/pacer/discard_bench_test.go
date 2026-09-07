package pacer_test

import (
	"testing"
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/pacer"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/utc-go"
)

// BenchmarkDiscardContext_ShouldDiscard_Complete measures the fast return path when the discard phase is already
// complete (DiscardComplete=true). This is the steady-state hot path for all packets after startup.
func BenchmarkDiscardContext_ShouldDiscard_Complete(b *testing.B) {
	dc := pacer.NewDiscardContext(0, 0, rtp.TicksToDuration)
	now := utc.MustParse("2000-01-01T12:00:00Z")
	// DiscardPeriod=0: first call completes the discard phase immediately.
	_, _ = dc.ShouldDiscard(0, now)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = dc.ShouldDiscard(0, now)
	}
}

// BenchmarkDiscardContext_ShouldDiscard_Active measures ShouldDiscard during the active discard phase, where T0 is
// stable and each call only needs to check whether the elapsed time has exceeded the discard period.
func BenchmarkDiscardContext_ShouldDiscard_Active(b *testing.B) {
	dc := pacer.NewDiscardContext(5*duration.S, 10*duration.S, rtp.TicksToDuration)
	t0 := utc.MustParse("2000-01-01T12:00:00Z")
	// Establish T0 baseline with the first packet.
	_, _ = dc.ShouldDiscard(0, t0)
	// One second in: baseline is stable but the 5s discard period has not elapsed yet.
	now := t0.Add(time.Second)
	ts := rtp.DurationToTicks(time.Second)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = dc.ShouldDiscard(ts, now)
	}
}
