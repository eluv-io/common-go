package mpegts

import (
	"testing"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/pacer"
	elog "github.com/eluv-io/log-go"
)

func BenchmarkTsDisruptorPacer_Push(b *testing.B) {
	const pid = 256
	const n = 96
	// NoPCR batch with no prior PCR: Push scans all packets finding no PCR and then discards the batch because no
	// target time has been established yet. This exercises the full PCR scan loop without blocking on the ring buffer.
	data := makeTsBatchNoPCR(pid, n)

	conf := TsDisruptorPacerConfig{
		Stream:        "bench",
		StatsLog:      elog.Noop,
		EventLog:      elog.Noop,
		StatsInterval: -1,
		Logic: pacer.PacerLogicConfig{
			DiscardPeriod:    duration.Duration(0),
			MaxDiscardPeriod: duration.Duration(0),
		},
	}
	p, err := NewTsDisruptorPacer(conf)
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = p.Push(data)
	}
	b.StopTimer()
	p.Shutdown()
}
