package pacer_test

import (
	"testing"
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/pacer"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

// BenchmarkPacerLogic_PacketTs measures PacketTs in steady state: no discard phase, baseline established, packets
// arriving at a consistent 10ms wall-clock and RTP interval with no drift. AdjustTimeDrift is disabled so that the
// benchmark measures the pure target-time computation path without drift correction branches.
func BenchmarkPacerLogic_PacketTs(b *testing.B) {
	stats := &pacer.InStats{}
	conf := pacer.PacerLogicConfig{
		Stream:          "bench",
		EventLog:        log.Get("/bench/pacer"),
		Delay:           duration.Spec(50 * time.Millisecond),
		AdjustTimeDrift: false,
		ToDuration:      rtp.TicksToDuration,
	}
	p := pacer.NewPacerLogic(conf, stats)

	t0 := utc.MustParse("2000-01-01T12:00:00Z")
	// DiscardPeriod=0: first call establishes the baseline immediately.
	_, _, _ = p.Packet(t0, 0, false)

	step := 10 * time.Millisecond
	stepTicks := rtp.DurationToTicks(step)
	now := t0.Add(step)
	ts := stepTicks

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = p.Packet(now, ts, false)
		now = now.Add(step)
		ts += stepTicks
	}
}
