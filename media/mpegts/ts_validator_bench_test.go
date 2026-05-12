package mpegts

import (
	"testing"
)

func BenchmarkTsValidator_Validate(b *testing.B) {
	const pid = 256
	const n = 96 // multiple of 16 so CCs stay consistent across repeated calls
	data := makeTsBatchConsistentCC(pid, 27_000_000, n)

	validator := NewTsValidator()
	// Warm up: establish PID/CC state.
	_ = validator.Validate(data)

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = validator.Validate(data)
	}
}
