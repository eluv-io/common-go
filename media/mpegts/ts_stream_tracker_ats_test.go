package mpegts

import (
	"encoding/binary"
	"testing"

	"github.com/Comcast/gots/v2/packet"
	"github.com/stretchr/testify/require"
)

// makeAtsTsInput frames a raw TS batch as an ATS-TS value: an 8-byte big-endian arrival timestamp followed by the TS
// packets.
func makeAtsTsInput(arrivalNs int64, batch []byte) []byte {
	out := make([]byte, AtsTimestampLen+len(batch))
	binary.BigEndian.PutUint64(out[:AtsTimestampLen], uint64(arrivalNs))
	copy(out[AtsTimestampLen:], batch)
	return out
}

// TestTsStreamTracker_AtsFramingEquivalence verifies that tracking an ATS-TS-framed input with TsFramingAts yields the
// same result as tracking the underlying raw TS batch with TsFramingNone: the arrival-timestamp prefix is stripped and
// otherwise does not affect tracking.
func TestTsStreamTracker_AtsFramingEquivalence(t *testing.T) {
	const pid = 256
	const nPackets = 7

	batch := makeTsBatch(pid, 900_000, nPackets)

	rawTracker := NewTsStreamTracker("raw", 0, false) // stripRtp=false → TsFramingNone
	rawCount, rawErr := rawTracker.Track(batch)

	atsTracker := NewTsStreamTrackerFramed("ats", 0, TsFramingAts)
	atsCount, atsErr := atsTracker.Track(makeAtsTsInput(1_700_000_000_000_000_000, batch))

	// Same packet count and same aggregated errors regardless of the ATS framing.
	require.Equal(t, rawCount, atsCount)
	require.Equal(t, rawErr != nil, atsErr != nil)

	rawStats, atsStats := rawTracker.Stats(), atsTracker.Stats()
	require.Equal(t, rawStats.PacketCount, atsStats.PacketCount)
	require.Equal(t, rawStats.ErrorCount, atsStats.ErrorCount)
	require.Len(t, atsStats.Streams, len(rawStats.Streams))
	for i := range rawStats.Streams {
		require.Equal(t, rawStats.Streams[i].Pid, atsStats.Streams[i].Pid)
		require.Equal(t, rawStats.Streams[i].PacketCount, atsStats.Streams[i].PacketCount)
	}
}

// TestTsStreamTracker_AtsFramingSinglePacket verifies clean tracking (no errors) of a single ATS-TS-framed TS packet and
// that the encapsulated stream's PID is discovered.
func TestTsStreamTracker_AtsFramingSinglePacket(t *testing.T) {
	const pid = 256

	tracker := NewTsStreamTrackerFramed("ats", 0, TsFramingAts)
	input := makeAtsTsInput(1_700_000_000_000_000_000, makeTsBatch(pid, 900_000, 1))

	count, err := tracker.Track(input)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	stats := tracker.Stats()
	require.Equal(t, 0, stats.ErrorCount)
	require.Equal(t, 1, stats.PacketCount)
	require.Len(t, stats.Streams, 1)
	require.Equal(t, pid, stats.Streams[0].Pid)
}

// TestTsStreamTracker_AtsFramingShortInput verifies that an input too short to contain the arrival-timestamp prefix is
// rejected.
func TestTsStreamTracker_AtsFramingShortInput(t *testing.T) {
	tracker := NewTsStreamTrackerFramed("ats", 0, TsFramingAts)
	_, err := tracker.Track([]byte{0x01, 0x02, 0x03}) // shorter than AtsTimestampLen
	require.Error(t, err)
}

// TestParseAtsTs verifies the ATS-TS value parser.
func TestParseAtsTs(t *testing.T) {
	payload := makeTsBatch(256, 900_000, 3)
	input := makeAtsTsInput(1_700_000_000_000_000_000, payload)

	arrivalNs, gotPayload, err := ParseAtsTs(input)
	require.NoError(t, err)
	require.Equal(t, int64(1_700_000_000_000_000_000), arrivalNs)
	require.Equal(t, 3*packet.PacketSize, len(gotPayload))
	require.Equal(t, payload, gotPayload)

	_, _, err = ParseAtsTs([]byte{0x00, 0x01})
	require.Error(t, err)
}
