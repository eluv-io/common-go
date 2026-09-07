package mpegts

import (
	"testing"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/pacer"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

func BenchmarkAtsDisruptorPacer_Push(b *testing.B) {
	const n = 96
	// A large DiscardPeriod (and disabled MaxDiscardPeriod) keeps the pacer in the discard phase for the whole
	// benchmark: after the first packet establishes the baseline, every subsequent Push parses the arrival timestamp
	// and is discarded, exercising the full parse path without blocking on the ring buffer.
	data := makeAtsPacket(utc.Now().UnixNano(), n)

	conf := AtsDisruptorPacerConfig{
		Stream:        "bench",
		StatsLog:      elog.Noop,
		EventLog:      elog.Noop,
		StatsInterval: -1,
		Logic: pacer.PacerLogicConfig{
			DiscardPeriod:    duration.H,
			MaxDiscardPeriod: 0, // disabled: never time out of the discard phase
		},
	}
	p, err := NewAtsDisruptorPacer(conf)
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
