package rtp

import (
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTicksToDuration_RoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		ticks    int64
		expected time.Duration
	}{
		{"zero", 0, 0},
		{"9 ticks", 9, 100 * time.Microsecond},
		{"90000 ticks (1s)", 90000, time.Second},
		{"900000 ticks (10s)", 900000, 10 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, TicksToDuration(tc.ticks))
		})
	}
}

// TestTicksToDuration_NoOverflow is a regression test for an int64 overflow in TicksToDuration's old implementation
// (time.Duration(ts) * 100 * time.Microsecond / 9), which multiplied before dividing: the intermediate ts*100000
// overflowed once ts exceeded math.MaxInt64/100000 (~9.22e13 ticks, ~32.5 years of continuous 90kHz RTP ticks) - a
// bound a long-running live stream's unwrapped (unbounded-across-wraps) RTP clock can reach. Verifies the result
// stays correct (cross-checked via arbitrary-precision arithmetic) well past that old threshold.
func TestTicksToDuration_NoOverflow(t *testing.T) {
	oldOverflowThreshold := int64(math.MaxInt64 / 100000)
	tests := []int64{
		oldOverflowThreshold,
		oldOverflowThreshold * 2,
		oldOverflowThreshold * 9,
	}
	for _, ts := range tests {
		got := TicksToDuration(ts)
		require.Positive(t, got, "ts=%d", ts)

		// Independent expected value via arbitrary-precision arithmetic: ts * 100 * 1000ns / 9, floored.
		expected := new(big.Int).Mul(big.NewInt(ts), big.NewInt(100*int64(time.Microsecond)))
		expected.Div(expected, big.NewInt(9))
		require.Equal(t, expected.Int64(), int64(got), "ts=%d", ts)
	}
}

// exactDurationToTicks computes floor(ts*9/100000) via arbitrary-precision arithmetic, independent of
// DurationToTicks's own implementation, as a correctness reference.
func exactDurationToTicks(ts time.Duration) int64 {
	exact := new(big.Int).Mul(big.NewInt(int64(ts)), big.NewInt(9))
	exact.Div(exact, big.NewInt(100000))
	return exact.Int64()
}

// TestDurationToTicks_MatchesExactFloor verifies DurationToTicks against the mathematically exact floor(ts*9/100000)
// - including at ts=11111, 111111, ... (every ts where ts*9 mod 100000 == 99999), which is a regression check for a
// real off-by-one bug in the pre-fix implementation's "+1" rounding term: it rounded up at exactly those values,
// independent of (and in addition to) the overflow bug fixed alongside it.
func TestDurationToTicks_MatchesExactFloor(t *testing.T) {
	for us := int64(0); us < 5_000_000; us++ {
		for _, rem := range []int64{0, 1, 499, 500, 999} {
			ts := time.Duration(us*int64(time.Microsecond) + rem)
			require.Equal(t, exactDurationToTicks(ts), DurationToTicks(ts), "ts=%d", ts)
		}
	}
}

// TestDurationToTicks_NoOverflow is a regression test for an int64 overflow in DurationToTicks's old implementation
// (ts*9+1 before any division), which overflowed once ts exceeded math.MaxInt64/9 (~32.5 years) - well within
// time.Duration's own representable range (~292 years).
func TestDurationToTicks_NoOverflow(t *testing.T) {
	oldOverflowThreshold := time.Duration(math.MaxInt64 / 9)
	tests := []time.Duration{
		oldOverflowThreshold,
		oldOverflowThreshold + oldOverflowThreshold/2,
		time.Duration(math.MaxInt64), // the largest representable time.Duration
	}
	for _, ts := range tests {
		got := DurationToTicks(ts)
		require.Positive(t, got, "ts=%d", ts)
		require.Equal(t, exactDurationToTicks(ts), got, "ts=%d", ts)
	}
}
