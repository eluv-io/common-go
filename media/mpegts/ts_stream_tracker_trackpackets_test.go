package mpegts

import (
	"testing"

	"github.com/Comcast/gots/v2/packet"
	"github.com/stretchr/testify/require"
)

// toPacketSlice splits a raw TS batch into individual *packet.Packet pointers aliasing batch's backing array, mirroring
// how pktpool.Packet.Ts().Packets() hands already-decoded TS packets to a caller.
func toPacketSlice(batch []byte) []*packet.Packet {
	n := len(batch) / packet.PacketSize
	pkts := make([]*packet.Packet, n)
	for i := range n {
		pkts[i] = (*packet.Packet)(batch[i*packet.PacketSize : (i+1)*packet.PacketSize])
	}
	return pkts
}

// TestTsStreamTracker_TrackPacketsEquivalence verifies that TrackPackets, given already-decoded packets (as a caller
// reading via pktpool.Packet.Ts().Packets() would produce), yields the same packet count and statistics as Track
// parsing the same raw bytes itself.
func TestTsStreamTracker_TrackPacketsEquivalence(t *testing.T) {
	const pid = 256
	const nPackets = 7

	batch := makeTsBatch(pid, 900_000, nPackets)

	byBytes := NewTsStreamTracker("by-bytes", 0, false)
	byBytesCount, byBytesErr := byBytes.Track(batch)

	byPackets := NewTsStreamTracker("by-packets", 0, false)
	byPacketsCount, byPacketsErr := byPackets.TrackPackets(toPacketSlice(batch))

	require.Equal(t, byBytesCount, byPacketsCount)
	require.Equal(t, byBytesErr, byPacketsErr)

	// Compare only the deterministic fields: Start/Duration are wall-clock based and will legitimately differ by the
	// few nanoseconds between the two Track/TrackPackets calls above.
	byBytesStats, byPacketsStats := byBytes.Stats(), byPackets.Stats()
	require.Equal(t, byBytesStats.PacketCount, byPacketsStats.PacketCount)
	require.Equal(t, byBytesStats.ErrorCount, byPacketsStats.ErrorCount)
	require.Len(t, byPacketsStats.Streams, len(byBytesStats.Streams))
	for i := range byBytesStats.Streams {
		require.Equal(t, byBytesStats.Streams[i].Pid, byPacketsStats.Streams[i].Pid)
		require.Equal(t, byBytesStats.Streams[i].PacketCount, byPacketsStats.Streams[i].PacketCount)
		require.Equal(t, byBytesStats.Streams[i].CcErrors, byPacketsStats.Streams[i].CcErrors)
	}
}

// TestTsStreamTracker_TrackPacketsDetectsContinuityError verifies that TrackPackets performs the same continuity-
// counter validation as Track: a corrupted continuity counter is reported as an error and reflected in Stats.
func TestTsStreamTracker_TrackPacketsDetectsContinuityError(t *testing.T) {
	batch := makeTsBatchConsistentCC(256, 900_000, 16)
	pkts := toPacketSlice(batch)

	// Corrupt the continuity counter of the last packet (lower nibble of byte 3) so it no longer follows the previous
	// one's.
	last := (*pkts[len(pkts)-1])[3]
	(*pkts[len(pkts)-1])[3] = last ^ 0x0F

	tracker := NewTsStreamTracker("test", 0, false)
	count, errList := tracker.TrackPackets(pkts)

	require.Equal(t, 16, count)
	require.Error(t, errList)

	stats := tracker.Stats()
	require.Equal(t, 1, stats.ErrorCount)
}

// TestTsStreamTracker_TrackPacketsTooManyErrors verifies TrackPackets aborts early (reporting the full packet count)
// once the per-call error threshold is exceeded, matching Track's behavior.
func TestTsStreamTracker_TrackPacketsTooManyErrors(t *testing.T) {
	n := 25
	pkts := make([]*packet.Packet, n)
	invalid := make([]byte, packet.PacketSize) // all-zero bytes: not a valid TS packet (bad sync byte)
	for i := range n {
		bts := make([]byte, packet.PacketSize)
		copy(bts, invalid)
		pkts[i] = (*packet.Packet)(bts)
	}

	tracker := NewTsStreamTracker("test", 0, false)
	count, errList := tracker.TrackPackets(pkts)

	require.Equal(t, n, count)
	require.Error(t, errList)
}
