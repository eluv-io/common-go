package mpegts

import (
	"testing"
)

func BenchmarkTsPacer_Wait(b *testing.B) {
	const pid = 256
	const n = 96
	// NoPCR batch: Wait scans all packets finding no PCR — exercises the full inner loop.
	data := makeTsBatchNoPCR(pid, n)

	p := NewTsPacer()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		p.Wait(data)
	}
}
