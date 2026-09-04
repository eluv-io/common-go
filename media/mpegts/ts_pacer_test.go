package mpegts

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTsPacer_PinsPcrToFirstPid verifies that the synchronous TsPacer pins PCR pacing to the first PID a PCR is
// detected on, and ignores PCRs on any other PID (other programs in a multi-program transport stream).
func TestTsPacer_PinsPcrToFirstPid(t *testing.T) {
	const (
		pinnedPid = 100
		otherPid  = 200
		tick10ms  = 270_000 // PCR ticks for 10ms at 27 MHz
	)

	p := NewTsPacer()

	// First batch: PCR on the pinned PID -> pins pacing to pinnedPid.
	p.Wait(makeTsBatch(pinnedPid, 100*tick10ms, 4))
	require.Equal(t, pinnedPid, p.pcrPid, "must pin to the first PID a PCR is seen on")
	require.Contains(t, p.pid2start, pinnedPid)

	// A batch whose only PCR is on a different program's PID must be ignored: the pin does not move and no start
	// reference is created for the other PID.
	p.Wait(makeTsBatch(otherPid, 900_000*tick10ms, 4))
	require.Equal(t, pinnedPid, p.pcrPid, "pinned PID must not change")
	require.NotContains(t, p.pid2start, otherPid, "PCR on a non-pinned PID must be ignored")

	// A mixed batch where the other program's PCR appears BEFORE the pinned PID's PCR: the pacer must skip the other
	// PID and pace on the pinned PID.
	mixed := append(append([]byte{},
		makeTsPacketWithPCR(otherPid, 900_000*tick10ms)...),
		makeTsPacketWithPCR(pinnedPid, 101*tick10ms)...)
	p.Wait(mixed)
	require.NotContains(t, p.pid2start, otherPid, "the other program's PCR must remain ignored")
}

// TestTsPacer_WaitProcessesFinalPacket is a regression test for the Wait() loop boundary: a PCR carried only by the
// last complete packet of a batch must still be processed (the guard used to be `len(bts) > PacketSize`, which
// skipped a trailing complete packet).
func TestTsPacer_WaitProcessesFinalPacket(t *testing.T) {
	const pid = 100

	p := NewTsPacer()
	// Two-packet batch with the PCR only on the last packet.
	batch := append(append([]byte{}, makeTsPacketNoPCR(pid)...), makeTsPacketWithPCR(pid, 270_000)...)
	p.Wait(batch)
	require.Equal(t, pid, p.pcrPid, "PCR on the final packet of a batch must be processed")
	require.Contains(t, p.pid2start, pid)
}
