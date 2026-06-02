package mpegts

import (
	"testing"

	"github.com/Comcast/gots/v2/packet"
)

// makeTsBatchConsistentCC builds n TS packets for the given pid where every packet has a payload and continuity
// counters increment by 1 each time (wrapping at 16). The first packet also carries a PCR in its adaptation field.
// n must be a multiple of 16 so that repeated calls leave stream.cc at 15, keeping the CC sequence consistent
// across Track calls without resetting.
func makeTsBatchConsistentCC(pid int, pcr uint64, n int) []byte {
	result := make([]byte, n*packet.PacketSize)
	for i := 0; i < n; i++ {
		pkt := packet.Create(pid, packet.WithHasPayloadFlag)
		if i == 0 {
			// Promote first packet to adaptation+payload so it can carry the PCR while still incrementing CC.
			if err := pkt.SetAdaptationFieldControl(packet.PayloadAndAdaptationFieldFlag); err == nil {
				if af, err := pkt.AdaptationField(); err == nil {
					_ = af.SetHasPCR(true)
					_ = af.SetPCR(pcr)
				}
			}
		}
		pkt.SetContinuityCounter(i % 16)
		copy(result[i*packet.PacketSize:], (*pkt)[:])
	}
	return result
}

func BenchmarkTsStreamTracker_Track(b *testing.B) {
	const pid = 256
	const n = 96 // multiple of 16 so CCs stay consistent across repeated calls
	data := makeTsBatchConsistentCC(pid, 27_000_000, n)

	tracker := NewTsStreamTracker("bench", 0, false)
	// Warm up: establish stream and PCR state so the first timed iteration starts with a known CC.
	_, _ = tracker.Track(data)
	tracker.Reset()

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = tracker.Track(data)
		tracker.Reset()
	}
}
