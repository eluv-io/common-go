package slidingwindow_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/collections/slidingwindow"
	"github.com/eluv-io/utc-go"
)

func TestSlidingWindow(t *testing.T) {
	clock := utc.NewWallClock(utc.UnixMilli(0))
	utc.MockNowClock(clock)
	defer clock.UnmockNow()

	sw := slidingwindow.New[int](5)

	// Add values to the sliding window
	add := func(v int) {
		sw.Add(v)
		clock.Add(time.Second)
	}

	add(1)
	add(2)
	split := clock.Now()
	add(3)
	add(4)
	add(5)

	// Check the mean and count
	require.Equal(t, 3.0, sw.Mean())
	require.Equal(t, 5, sw.Count())

	{
		// validate stats
		sts := sw.Statistics()
		require.Equal(t, 3.0, sts.Mean())
		require.Equal(t, 1, sts.Min())
		require.Equal(t, 5, sts.Max())
		require.Equal(t, 0, sts.Quantile(-1.0))
		require.Equal(t, 0, sts.Quantile(1.1))
		require.Equal(t, 1, sts.Quantile(0.0))
		require.Equal(t, 2, sts.Quantile(0.1))
		require.Equal(t, 2, sts.Quantile(0.25))
		require.Equal(t, 3, sts.Quantile(0.5))
		require.Equal(t, 4, sts.Quantile(0.75))
		require.Equal(t, 5, sts.Quantile(1.0))

		// ensure progressive variance matches full variance
		require.Equal(t, sts.Variance(), sw.Variance())
	}

	{
		// validate stats of subset
		sts := sw.Statistics(split)
		require.Equal(t, 3, sts.Count())
		require.Equal(t, 4.0, sts.Mean())
		require.Equal(t, 3, sts.Min())
		require.Equal(t, 5, sts.Max())
		require.Equal(t, 0, sts.Quantile(-1.0))
		require.Equal(t, 0, sts.Quantile(1.1))
		require.Equal(t, 3, sts.Quantile(0.0))
		require.Equal(t, 4, sts.Quantile(0.5))
		require.Equal(t, 5, sts.Quantile(1.0))
	}

	// Add another value, which should replace the oldest value (1)
	add(6)
	require.Equal(t, sw.Statistics().Variance(), sw.Variance())

	// Check the mean and count again
	require.Equal(t, 4.0, sw.Mean())
	require.Equal(t, 5, sw.Count())
	require.Equal(t, sw.Statistics().Variance(), sw.Variance())

	// Add more values to test circular behavior
	add(7)
	add(8)

	// Check the mean after adding more values
	require.Equal(t, 6.0, sw.Mean())
	require.Equal(t, sw.Statistics().Variance(), sw.Variance())

	// Fill up with 1-5 again...
	add(1)
	add(2)
	add(3)
	add(4)
	add(5)

	// Check the mean and count
	require.Equal(t, 3.0, sw.Mean())
	require.Equal(t, 5, sw.Count())
	require.InDelta(t, sw.Statistics().Variance(), sw.Variance(), 0.0001, "Variance should match the full variance calculation")
}

func ExampleSlidingWindow() {
	sw := slidingwindow.New[float64](3)
	sw.Add(10)
	sw.Add(20)
	sw.Add(30)
	sw.Add(40) // slides window, now contains 20, 30, 40

	stats := sw.Statistics()
	fmt.Printf("Count:    %d\n", stats.Count())
	fmt.Printf("Mean:     %.1f\n", stats.Mean())
	fmt.Printf("Variance: %.1f\n", stats.Variance())
	fmt.Printf("Min:      %.1f\n", stats.Min())
	fmt.Printf("Max:      %.1f\n", stats.Max())
	fmt.Printf("Median:   %.1f\n", stats.Quantile(0.5))

	// Output:
	// Count:    3
	// Mean:     30.0
	// Variance: 100.0
	// Min:      20.0
	// Max:      40.0
	// Median:   30.0
}
