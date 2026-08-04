package rtp_test

import (
	"testing"
	"time"

	pionrtp "github.com/pion/rtp"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/media/rtp"
)

// marshalRtpPacket builds and marshals a single RTP packet with the given sequence number, timestamp, and payload.
func marshalRtpPacket(t *testing.T, seq uint16, ts uint32, payload []byte) []byte {
	t.Helper()
	pkt := pionrtp.Packet{
		Header: pionrtp.Header{
			Version:        2,
			SequenceNumber: seq,
			Timestamp:      ts,
		},
		Payload: payload,
	}
	bts, err := pkt.Marshal()
	require.NoError(t, err)
	return bts
}

// TestRtpStreamTracker_TrackPacketEquivalence verifies that TrackPacket, given an already-parsed packet (as a caller
// reading via pktpool.Packet.Rtp().Packet() would produce), yields the same payload and statistics as Track parsing
// the same bytes itself.
func TestRtpStreamTracker_TrackPacketEquivalence(t *testing.T) {
	bts1 := marshalRtpPacket(t, 1, 1000, []byte{1, 2, 3})
	bts2 := marshalRtpPacket(t, 2, 1090, []byte{4, 5, 6})

	byBytes := rtp.NewStreamTracker("by-bytes", 0, 10, time.Second)
	byPacket := rtp.NewStreamTracker("by-packet", 0, 10, time.Second)

	for _, bts := range [][]byte{bts1, bts2} {
		payloadFromBytes, _, errFromBytes := byBytes.Track(bts)

		pkt, err := rtp.ParsePacket(bts)
		require.NoError(t, err)
		payloadFromPacket, _, errFromPacket := byPacket.TrackPacket(pkt)

		require.Equal(t, payloadFromBytes, payloadFromPacket)
		require.Equal(t, errFromBytes, errFromPacket)
	}

	// Compare only the deterministic fields: Start/Duration/TsAdjDuration are wall-clock based and will legitimately
	// differ by the few nanoseconds between the two Track/TrackPacket calls above.
	byBytesStats, byPacketStats := byBytes.Stats(), byPacket.Stats()
	require.Equal(t, byBytesStats.PacketCount, byPacketStats.PacketCount)
	require.Equal(t, byBytesStats.ErrorCount, byPacketStats.ErrorCount)
	require.Equal(t, byBytesStats.StartSeq, byPacketStats.StartSeq)
	require.Equal(t, byBytesStats.EndSeq, byPacketStats.EndSeq)
	require.Equal(t, byBytesStats.StartTs, byPacketStats.StartTs)
	require.Equal(t, byBytesStats.EndTs, byPacketStats.EndTs)
	require.Equal(t, byBytesStats.Gaps, byPacketStats.Gaps)
}

// TestRtpStreamTracker_TrackPacketDetectsGap verifies that TrackPacket performs the same sequence-gap detection as
// Track: a skipped sequence number is reported as an error and recorded in Stats.Gaps.
func TestRtpStreamTracker_TrackPacketDetectsGap(t *testing.T) {
	tracker := rtp.NewStreamTracker("test", 0, 1, time.Second)

	pkt1, err := rtp.ParsePacket(marshalRtpPacket(t, 1, 1000, []byte{1}))
	require.NoError(t, err)
	_, _, err = tracker.TrackPacket(pkt1)
	require.NoError(t, err)

	pkt2, err := rtp.ParsePacket(marshalRtpPacket(t, 5, 1090, []byte{2})) // skips 2, 3, 4
	require.NoError(t, err)
	_, _, err = tracker.TrackPacket(pkt2)
	require.Error(t, err)

	stats := tracker.Stats()
	require.Equal(t, 2, stats.PacketCount)
	require.Equal(t, 1, stats.ErrorCount)
	require.Len(t, stats.Gaps, 1)
}
