package rtp_test

import (
	"testing"
	"time"

	"github.com/eluv-io/common-go/media/rtp"
)

// BenchmarkReorderBuffer_InOrder measures the steady-state cost of Push when every packet arrives exactly in
// order - the common case, and the one the fast-path skip in Push's delta==0 branch is meant to help.
func BenchmarkReorderBuffer_InOrder(b *testing.B) {
	buf := rtp.NewReorderBuffer[int](32, 20*time.Millisecond, 0)
	now := time.Now()
	out := make([]int, 0, 1)

	b.ReportAllocs()
	b.ResetTimer()

	seq := uint16(0)
	for b.Loop() {
		out = buf.Push(now, seq, int(seq), out[:0])
		seq++
	}
}

// BenchmarkReorderBuffer_OutOfOrderRecovery measures the cost of correcting a steady stream of single-pair swaps
// (seq n+1 arrives before seq n, for every pair) - the buffer's actual reason to exist.
func BenchmarkReorderBuffer_OutOfOrderRecovery(b *testing.B) {
	buf := rtp.NewReorderBuffer[int](32, 20*time.Millisecond, 0)
	now := time.Now()
	out := make([]int, 0, 2)

	b.ReportAllocs()
	b.ResetTimer()

	seq := uint16(0)
	for b.Loop() {
		// Push seq+1 first (held), then seq (fills the gap and cascades seq+1 out) - one corrected swap per
		// iteration.
		out = buf.Push(now, seq+1, int(seq+1), out[:0])
		out = buf.Push(now, seq, int(seq), out[:0])
		seq += 2
	}
}
