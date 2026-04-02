package mpegts

import (
	"testing"
	"time"

	"github.com/Comcast/gots/v2"
	"github.com/Comcast/gots/v2/packet"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
)

func TestPcrToDuration_RoundTrip(t *testing.T) {
	// These cases all use tick counts divisible by 27, so the DurationToPcr round-trip is exact.
	tests := []struct {
		name     string
		ticks    uint64
		expected time.Duration
	}{
		{"zero", 0, 0},
		{"1µs", 27, time.Microsecond},
		{"1s", 27_000_000, time.Second},
		{"100s", 2_700_000_000, 100 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dur := PcrToDuration(tc.ticks)
			ticks := DurationToPcr(dur)
			require.Equal(t, tc.expected, dur)
			require.Equal(t, tc.ticks, ticks)
		})
	}

	// MaxPCR is not divisible by 27, so the round-trip has ±1 tick precision loss.
	t.Run("MaxPCR", func(t *testing.T) {
		maxPCRdur := duration.MustParse("26h30m43.717696703s").Duration()
		require.Equal(t, maxPCRdur, PcrToDuration(MaxPCR))
		require.Equal(t, uint64(MaxPCR-1), DurationToPcr(maxPCRdur))
	})
}

func TestPtsToDuration(t *testing.T) {
	tests := []struct {
		name     string
		ticks    uint64
		expected time.Duration
	}{
		{"zero", 0, 0},
		{"100µs", 9, 100 * time.Microsecond},
		{"1s", 90_000, time.Second},
		{"10s", 900_000, 10 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, PtsToDuration(tc.ticks))
		})
	}
}

func TestPcrDiff(t *testing.T) {
	tests := []struct {
		name     string
		p1, p2   uint64
		expected time.Duration
	}{
		{"equal at zero", 0, 0, 0},
		{"equal at max", MaxPCR, MaxPCR, 0},
		{"normal 1s", 0, 27_000_000, time.Second},
		{"normal reverse is wraparound", 27_000_000, 0, PcrToDuration(MaxPCR - 27_000_000 + 1)},
		{"single-tick wraparound", MaxPCR, 0, PcrToDuration(1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, PcrDiff(tc.p1, tc.p2))
		})
	}
}

func TestExtractPCR(t *testing.T) {
	t.Run("no adaptation field", func(t *testing.T) {
		pkt := packet.New()
		_, ok := ExtractPCR(pkt)
		require.False(t, ok)
	})

	t.Run("has PCR", func(t *testing.T) {
		const wantPCR = uint64(27_000_000)
		pktBytes := makeTsPacketWithPCR(256, wantPCR)
		pkt := packet.Packet(pktBytes)
		pcr, ok := ExtractPCR(&pkt)
		require.True(t, ok)
		require.Equal(t, wantPCR, pcr)
	})
}

// makePESPayloadPTSOnly returns a minimal PES payload carrying only a PTS value.
func makePESPayloadPTSOnly(pts uint64) []byte {
	b := make([]byte, 14)
	b[0], b[1], b[2] = 0x00, 0x00, 0x01 // start code prefix
	b[3] = 0xE0                         // video stream ID
	b[4], b[5] = 0x00, 0x00             // PES packet length (0 = unspecified for video)
	b[6] = 0x80                         // marker bits
	b[7] = 0x80                         // PTS_DTS_flags = 10 (PTS only)
	b[8] = 0x05                         // PES header data length = 5 bytes
	gots.InsertPTS(b[9:14], pts)
	return b
}

// makePESPayloadPTSAndDTS returns a minimal PES payload carrying both PTS and DTS values.
func makePESPayloadPTSAndDTS(pts, dts uint64) []byte {
	b := make([]byte, 19)
	b[0], b[1], b[2] = 0x00, 0x00, 0x01 // start code prefix
	b[3] = 0xE0                         // video stream ID
	b[4], b[5] = 0x00, 0x00             // PES packet length
	b[6] = 0x80                         // marker bits
	b[7] = 0xC0                         // PTS_DTS_flags = 11 (both PTS and DTS)
	b[8] = 0x0A                         // PES header data length = 10 bytes (5 PTS + 5 DTS)
	gots.InsertPTS(b[9:14], pts)
	gots.InsertPTS(b[14:19], dts)
	return b
}

func TestExtractPTS(t *testing.T) {
	t.Run("no PUSI", func(t *testing.T) {
		pkt := packet.New()
		pts, dts, ok := ExtractPTS(pkt)
		require.False(t, ok)
		require.Zero(t, pts)
		require.Zero(t, dts)
	})

	t.Run("PUSI set invalid payload", func(t *testing.T) {
		pkt := packet.New()
		pkt.SetPayloadUnitStartIndicator(true)
		_, err := pkt.SetPayload([]byte{0xFF, 0xFF})
		require.NoError(t, err)
		_, _, ok := ExtractPTS(pkt)
		require.False(t, ok)
	})

	t.Run("PTS only", func(t *testing.T) {
		const wantPTS = uint64(90_000)
		pkt := packet.New()
		pkt.SetPayloadUnitStartIndicator(true)
		_, err := pkt.SetPayload(makePESPayloadPTSOnly(wantPTS))
		require.NoError(t, err)
		pts, dts, ok := ExtractPTS(pkt)
		require.True(t, ok)
		require.Equal(t, wantPTS, pts)
		require.Equal(t, wantPTS, dts) // DTS defaults to PTS when absent
	})

	t.Run("PTS and DTS", func(t *testing.T) {
		const wantPTS = uint64(90_000)
		const wantDTS = uint64(86_997)
		pkt := packet.New()
		pkt.SetPayloadUnitStartIndicator(true)
		_, err := pkt.SetPayload(makePESPayloadPTSAndDTS(wantPTS, wantDTS))
		require.NoError(t, err)
		pts, dts, ok := ExtractPTS(pkt)
		require.True(t, ok)
		require.Equal(t, wantPTS, pts)
		require.Equal(t, wantDTS, dts)
	})
}
