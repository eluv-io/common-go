package mpegts

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestPcrUnwrapper_FirstCall verifies that the first call establishes baseline and returns a synthetic previous value.
func TestPcrUnwrapper_FirstCall(t *testing.T) {
	var u PcrUnwrapper
	prev, curr := u.Unwrap(1000)
	require.Equal(t, int64(999), prev) // synthetic previous = current - 1
	require.Equal(t, int64(1000), curr)
}

// TestPcrUnwrapper_ForwardStep verifies normal monotone advancement without wraparound.
func TestPcrUnwrapper_ForwardStep(t *testing.T) {
	var u PcrUnwrapper
	u.Unwrap(1000)
	prev, curr := u.Unwrap(2000)
	require.Equal(t, int64(1000), prev)
	require.Equal(t, int64(2000), curr)
}

// TestPcrUnwrapper_ForwardWraparound verifies that a PCR counter wrap-around from near MaxPCR back to near 0
// is treated as a forward step rather than a large backward jump.
func TestPcrUnwrapper_ForwardWraparound(t *testing.T) {
	var u PcrUnwrapper

	// Establish baseline near MaxPCR.
	nearMax := uint64(MaxPCR - 100)
	prev, curr := u.Unwrap(nearMax)
	require.Equal(t, int64(nearMax)-1, prev) // synthetic previous
	require.Equal(t, int64(nearMax), curr)

	// Advance slightly within normal range.
	prev, curr = u.Unwrap(nearMax + 50)
	require.Equal(t, int64(nearMax), prev)
	require.Equal(t, int64(nearMax)+50, curr)

	// Wrap: PCR resets to near 0. The unwrapped counter must continue monotonically.
	// diff = 50 - (nearMax+50) = -(MaxPCR-100), which is < -halfRange → add MaxPCR+1 → diff = 101.
	prev, curr = u.Unwrap(50)
	require.Equal(t, int64(nearMax)+50, prev)
	require.Equal(t, int64(nearMax)+50+101, curr)

	// Next normal forward step after wraparound.
	prev, curr = u.Unwrap(151)
	require.Equal(t, int64(nearMax)+50+101, prev)
	require.Equal(t, int64(nearMax)+50+101+101, curr) // diff = 151 - 50 = 101
}

// TestPcrUnwrapper_BackwardWraparound verifies that a backward wraparound (PCR jumps from near 0 to near MaxPCR,
// which can happen on a stream restart or counter reset) results in a small backward step in the monotonic sequence
// rather than a large forward jump.
func TestPcrUnwrapper_BackwardWraparound(t *testing.T) {
	var u PcrUnwrapper

	// Establish baseline near 0.
	u.Unwrap(50)

	// PCR jumps to near MaxPCR: diff = (MaxPCR-50) - 50 = MaxPCR-100, which exceeds halfRange.
	// Algorithm subtracts MaxPCR+1 → diff = (MaxPCR-100) - (MaxPCR+1) = -101.
	prev, curr := u.Unwrap(uint64(MaxPCR - 50))
	require.Equal(t, int64(50), prev)
	require.Equal(t, int64(50)-101, curr) // small backward step, not a giant leap forward
}

// TestPcrGapDetector_FirstCallNeverGap verifies that the very first PCR is never considered a gap.
func TestPcrGapDetector_FirstCallNeverGap(t *testing.T) {
	gd := PcrGapDetector{Threshold: DurationToPcr(time.Second)}
	_, _, gap := gd.Detect(1000)
	require.False(t, gap)
}

// TestPcrGapDetector_NormalStep verifies that a step within threshold is not flagged as a gap.
func TestPcrGapDetector_NormalStep(t *testing.T) {
	gd := PcrGapDetector{Threshold: DurationToPcr(time.Second)}
	gd.Detect(1000)
	_, _, gap := gd.Detect(2000)
	require.False(t, gap)
}

// TestPcrGapDetector_LargeJump verifies that a delta exceeding the threshold is flagged as a gap.
func TestPcrGapDetector_LargeJump(t *testing.T) {
	threshold := DurationToPcr(time.Second)
	gd := PcrGapDetector{Threshold: threshold}
	gd.Detect(1000)
	bigJump := uint64(1000) + uint64(threshold) + 1
	_, _, gap := gd.Detect(bigJump)
	require.True(t, gap)
}

// TestPcrGapDetector_NormalStepAfterGap verifies that a normal step right after a gap is not itself a gap.
func TestPcrGapDetector_NormalStepAfterGap(t *testing.T) {
	threshold := DurationToPcr(time.Second)
	gd := PcrGapDetector{Threshold: threshold}
	gd.Detect(1000)
	bigJump := uint64(1000) + uint64(threshold) + 1
	gd.Detect(bigJump)
	_, _, gap := gd.Detect(bigJump + 1000)
	require.False(t, gap)
}

// TestPcrGapDetector_ZeroThreshold verifies that zero threshold disables gap detection entirely.
func TestPcrGapDetector_ZeroThreshold(t *testing.T) {
	gd := PcrGapDetector{Threshold: 0}
	gd.Detect(0)
	_, _, gap := gd.Detect(uint64(MaxPCR)) // enormous jump
	require.False(t, gap, "zero threshold must never report a gap")
}
