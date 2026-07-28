package duration_test

// This test lives in the external duration_test package (not in millis_test.go, which is package duration) because
// statsutil imports duration - an internal-package test importing statsutil would form an import cycle.

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/statsutil"
	"github.com/eluv-io/utc-go"
)

// TestStatistics_Millis verifies that a statsutil.Statistics parameterized with duration.Millis reports and marshals
// all of its value fields - including the Mean - in the same millis unit. The mean is accumulated in float64 internally
// and converted to Millis only for reporting, so a fractional-millisecond average (25.25ms here) is represented
// exactly instead of being truncated.
func TestStatistics_Millis(t *testing.T) {
	now := utc.Now()
	stats := statsutil.Statistics[duration.Millis]{}
	for _, v := range []duration.Millis{
		duration.Millis(10 * time.Millisecond),
		duration.Millis(20 * time.Millisecond),
		duration.Millis(30 * time.Millisecond),
		duration.Millis(41 * time.Millisecond),
	} {
		stats.Update(now, v)
	}

	require.EqualValues(t, 4, stats.Count)
	require.Equal(t, duration.Millis(10*time.Millisecond), stats.Min)
	require.Equal(t, duration.Millis(41*time.Millisecond), stats.Max)
	require.Equal(t, duration.Millis(101*time.Millisecond), stats.Sum)
	// mean = 101ms / 4 = 25.25ms, represented exactly as Millis
	require.Equal(t, duration.Millis(25*time.Millisecond+250*time.Microsecond), stats.Mean)

	// Raw() marshals only the value fields; the mean renders as a millis number (25.250) in the same unit as
	// min/max/sum - not as raw nanoseconds and not truncated to a whole millisecond.
	marshaled, err := json.Marshal(stats.Raw())
	require.NoError(t, err)
	require.Equal(t, `{"count":4,"min":10.000,"max":41.000,"sum":101.000,"mean":25.250}`, string(marshaled))
}
