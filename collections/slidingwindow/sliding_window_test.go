package slidingwindow_test

import (
	"fmt"
	"math"
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

	{
		// validate stats
		sts := sw.Statistics()
		require.Equal(t, 0.0, sts.Mean())
		require.Equal(t, 0.0, sts.Variance())
		require.Equal(t, 0.0, sts.Stddev())
		require.Equal(t, 0, sts.Min())
		require.Equal(t, 0, sts.Max())
		require.Equal(t, 0, sts.Quantile(-1.0))
		require.Equal(t, 0, sts.Quantile(1.1))
		require.Equal(t, 0, sts.Quantile(0.0))
		require.Equal(t, 0, sts.Quantile(0.5))
		require.Equal(t, 0, sts.Quantile(1.0))
	}

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
		require.Equal(t, 2.0, sts.Variance())
		require.Equal(t, math.Sqrt(2.0), sts.Stddev())
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

	{
		// validate stats of empty subset
		sts := sw.Statistics(clock.Now())
		require.Equal(t, 0, sts.Count())
		require.Equal(t, 0.0, sts.Mean())
		require.Equal(t, 0.0, sts.Variance())
		require.Equal(t, 0.0, sts.Stddev())
		require.Equal(t, 0, sts.Min())
		require.Equal(t, 0, sts.Max())
		require.Equal(t, 0, sts.Quantile(0.5))
		require.Equal(t, 0, sts.Quantile(1.0))
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

func TestSlidingWindow_Variance(t *testing.T) {
	sw := slidingwindow.New[time.Duration](3)
	require.Zero(t, sw.Variance())
	require.Zero(t, sw.Statistics().Variance())

	sw.UseSampleVariance()
	require.Zero(t, sw.Variance())
	require.Zero(t, sw.Statistics().Variance())

	sw.Add(2)

	sw.UsePopulationVariance()
	require.Zero(t, sw.Variance())
	require.Zero(t, sw.Statistics().Variance())

	sw.UseSampleVariance()
	require.Zero(t, sw.Variance())
	require.Zero(t, sw.Statistics().Variance())

	sw.Add(4)
	sw.Add(6)

	sw.UsePopulationVariance()
	require.Equal(t, 4.0, sw.Mean())
	require.Equal(t, 8.0/3.0, sw.Variance())
	require.Equal(t, 8.0/3.0, sw.Statistics().Variance())

	sw.UseSampleVariance()
	require.Equal(t, 4.0, sw.Variance())
	require.Equal(t, 4.0, sw.Statistics().Variance())
}

func ExampleSlidingWindow() {
	sw := slidingwindow.New[float64](5)
	sw.Add(10)
	sw.Add(20)
	sw.Add(30)
	sw.Add(40)
	sw.Add(50)
	sw.Add(60) // slides window, now contains 20, 30, 40, 50, 60

	stats := sw.Statistics()
	fmt.Printf("Count:    %d\n", stats.Count())
	fmt.Printf("Mean:     %.1f\n", stats.Mean())
	fmt.Printf("Variance: %.1f (population)\n", stats.Variance())
	fmt.Printf("Min:      %.1f\n", stats.Min())
	fmt.Printf("Max:      %.1f\n", stats.Max())
	fmt.Printf("Median:   %.1f\n", stats.Quantile(0.5))
	fmt.Println()

	stats = sw.UseSampleVariance().Statistics()
	fmt.Printf("Variance: %.1f (sample)\n", stats.Variance())

	// Output:
	//
	// Count:    5
	// Mean:     40.0
	// Variance: 200.0 (population)
	// Min:      20.0
	// Max:      60.0
	// Median:   40.0
	//
	// Variance: 250.0 (sample)
}
