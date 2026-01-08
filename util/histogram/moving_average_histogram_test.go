package histogram

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCountToKeep(t *testing.T) {
	type testCase struct {
		durationPerHistogram time.Duration
		toCover              time.Duration
		wantCount            int
	}
	for i, tc := range []*testCase{
		{durationPerHistogram: time.Second * 1, toCover: time.Minute, wantCount: 61},
		{durationPerHistogram: time.Second * 5, toCover: time.Minute, wantCount: 13},
		{durationPerHistogram: time.Second * 10, toCover: time.Minute, wantCount: 7},
		{durationPerHistogram: time.Second * 20, toCover: time.Minute, wantCount: 4},
		{durationPerHistogram: time.Second * 29, toCover: time.Minute, wantCount: 4},
		{durationPerHistogram: time.Second * 30, toCover: time.Minute, wantCount: 3},
		{durationPerHistogram: time.Minute * 1, toCover: time.Minute, wantCount: 2},
		{durationPerHistogram: time.Minute + 1, toCover: time.Minute, wantCount: 2},
		{durationPerHistogram: time.Minute * 2, toCover: time.Minute, wantCount: 2},
		{durationPerHistogram: time.Minute * 3, toCover: time.Minute, wantCount: 2},
		{durationPerHistogram: time.Minute*3 + 1, toCover: time.Minute, wantCount: 2},
		{durationPerHistogram: time.Minute * 4, toCover: time.Minute, wantCount: 2},
		{durationPerHistogram: time.Second * 1, toCover: time.Hour, wantCount: 3601},
		{durationPerHistogram: time.Second * 5, toCover: time.Hour, wantCount: 721},
		{durationPerHistogram: time.Minute * 1, toCover: time.Hour, wantCount: 61},
		{durationPerHistogram: time.Minute * 5, toCover: time.Hour, wantCount: 13},
	} {
		ck := countToKeep(tc.durationPerHistogram, tc.toCover)
		require.Equal(t, tc.wantCount, ck, "failed case %d at %v / %v - expected %d, got %d", i, tc.toCover, tc.durationPerHistogram, tc.wantCount, ck)
	}
}

func TestNewMovingAverageHistogram(t *testing.T) {
	createHistogram := func() *MovingAverageHistogram[time.Duration] {
		histogram, err := NewMovingAverageHistogram(func() *Histogram[time.Duration] {
			return NewDurationHistogram(SegmentLatencyHistogram)
		}, 10, 10*time.Second)
		require.NoError(t, err)
		return histogram
	}
	avgFn := func(h *Histogram[time.Duration]) time.Duration {
		return h.Average()
	}

	t.Run("identical values per rotation", func(t *testing.T) {
		histogram := createHistogram()
		for i := 0; i < 10; i++ {
			histogram.Observe(100 * time.Millisecond)
			histogram.Observe(100 * time.Millisecond)
			histogram.Observe(200 * time.Millisecond)
			histogram.Observe(200 * time.Millisecond)

			avg := histogram.StatLastMinute(avgFn)
			require.Equal(t, 150*time.Millisecond, avg)

			histogram.Rotate()
		}
	})

	t.Run("increasing values per rotation", func(t *testing.T) {
		histogram := createHistogram()
		want := []time.Duration{
			0 * time.Millisecond,   // 0
			50 * time.Millisecond,  // 0+100
			100 * time.Millisecond, // 0+100+200
			150 * time.Millisecond, // 0+100+200+300
			200 * time.Millisecond, // 0+100+200+300+400
			250 * time.Millisecond, // 0+100+200+300+400+500
			300 * time.Millisecond, // 0+100+200+300+400+500+600
			400 * time.Millisecond, // 100+200+300+400+500+600+700 | +700
			500 * time.Millisecond, // 200+300+400+500+600+700+800 | +700
			600 * time.Millisecond, // 300+400+500+600+700+800+900 | +700
		}
		for i := 0; i < 10; i++ {
			histogram.Observe(time.Duration(i) * 100 * time.Millisecond)

			ck := countToKeep(10*time.Second, time.Minute)
			require.Equal(t, 7, ck)

			avg := histogram.StatLastMinute(avgFn)
			fmt.Println(avg)
			require.Equal(t, want[i], avg)

			histogram.Rotate()
		}
	})

}
