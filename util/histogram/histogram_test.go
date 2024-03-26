package histogram

import (
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDurationHistogramBounds(t *testing.T) {
	h := NewDefaultDurationHistogram()
	for i, b := range h.bins {
		//fmt.Println(b.label)
		lims := strings.Split(b.Label, "-")
		require.Equal(t, 2, len(lims))
		d0, err := time.ParseDuration(lims[0])
		require.NoError(t, err)
		d1, err := time.ParseDuration(lims[1])
		if i < len(h.bins)-1 {
			require.NoError(t, err)
		} else if err != nil {
			d1 = time.Duration(math.MaxInt64)
		}
		require.True(t, d0 < d1, "For %s: %s >= %s", b.Label, d0.String(), d1.String())
		if i > 0 {
			require.Equal(t, h.bins[i-1].Max, d0)
		}
		if i < len(h.bins)-1 {
			require.Equal(t, b.Max, d1)
		}
	}
}

func TestBinsValidation(t *testing.T) {
	emptyBins := []*DurationBin{}
	unboundedBeforeEnd := []*DurationBin{
		{Label: "0-10", Max: 10},
		{Label: "10-", Max: 0},
		{Label: "10-20", Max: 20},
	}
	duplicateBins := []*DurationBin{
		{Label: "0-10", Max: 10},
		{Label: "10-20", Max: 20},
		{Label: "10-20", Max: 20},
	}
	decreasingBins := []*DurationBin{
		{Label: "0-10", Max: 10},
		{Label: "15-20", Max: 20},
		{Label: "10-15", Max: 15},
	}
	notEmptyBin := []*DurationBin{
		{Label: "0-10", Max: 10},
		{Label: "10-20", Max: 20, Count: 15},
	}

	testCases := [][]*DurationBin{emptyBins, unboundedBeforeEnd, duplicateBins, decreasingBins, notEmptyBin}
	for _, testCase := range testCases {
		_, err := NewDurationHistogramBins(testCase)
		require.Error(t, err)
	}
}

func TestDurationHistogram(t *testing.T) {
	h := NewDefaultDurationHistogram()

	for i, b := range h.bins {
		min := time.Duration(0)
		if i > 0 {
			min = h.bins[i-1].Max
		}
		h.Observe(min)
		if b.Max != 0 {
			h.Observe(b.Max)
			h.Observe(b.Max + 1)
		}
	}
	for i, b := range h.bins {
		if i == len(h.bins)-1 {
			require.Equal(t, int64(1), b.Count)
		} else {
			require.Equal(t, int64(3), b.Count)
		}
		switch i {
		case 0:
			require.EqualValues(t, b.Max*2, b.DSum)
		case len(h.bins) - 1:
			require.EqualValues(t, b.Max*2+h.bins[i-1].Max+1, b.DSum)
		default:
			require.EqualValues(t, b.Max*2+h.bins[i-1].Max+1, b.DSum)
		}
	}
}

func TestDurationMaxBehavior(t *testing.T) {
	bins := []*DurationBin{
		{Label: "0-10", Max: 10},
		{Label: "10-"},
	}

	h, err := NewDurationHistogramBins(bins)
	require.NoError(t, err)

	h.Observe(10)
	h.Observe(11)
	h.Observe(100)
	require.Equal(t, int64(2), h.bins[1].Count)
	require.Equal(t, int64(111), h.bins[1].DSum)

	bins2 := []*DurationBin{
		{Label: "0-10", Max: 10},
		{Label: "10-20", Max: 20},
	}

	h2, err := NewDurationHistogramBins(bins2)
	require.NoError(t, err)

	h2.Observe(10)
	h2.Observe(11)
	h2.Observe(100)
	require.Equal(t, int64(1), h2.bins[1].Count)
	require.Equal(t, int64(11), h2.bins[1].DSum)
}

func TestStandardDeviation(t *testing.T) {
	bins := []*DurationBin{
		{Label: "0-10", Max: 10},
		{Label: "10-50", Max: 50},
		{Label: "50-90", Max: 90},
		{Label: "90-100", Max: 100},
	}
	h, err := NewDurationHistogramBins(bins)
	require.NoError(t, err)

	data := []time.Duration{
		3, 7, 12, 13, 14, 15, 15, 15, 16, 17, 25, 30, 60, 70, 80, 85, 93,
	}

	for _, d := range data {
		h.Observe(d)
	}

	require.Equal(t, time.Duration(29), h.StandardDeviation())
}

func TestStandardDeviation2(t *testing.T) {
	values := []*SerializedDurationBin{
		{Label: "0-100ms", Count: 11917, DSum: 750548874146},
		{Label: "100ms-200ms", Count: 11174, DSum: 1632240504452},
		{Label: "200ms-300ms", Count: 6794, DSum: 1668223282733},
		{Label: "300ms-400ms", Count: 3911, DSum: 1350261250114},
		{Label: "400ms-500ms", Count: 2449, DSum: 1091202961948},
		{Label: "500ms-750ms", Count: 2644, DSum: 1585670182188},
		{Label: "750ms-1s", Count: 781, DSum: 665865316657},
		{Label: "1s-2s", Count: 328, DSum: 389033374409},
		{Label: "2s-4s", Count: 2, DSum: 4031693641},
		{Label: "4s-", Count: 0, DSum: 0},
	}
	h := NewDurationHistogram(SegmentLatencyHistogram)
	h.LoadValues(values)

	require.Equal(t, int64(196), h.StandardDeviation().Milliseconds())
}
func TestQuantileEstimation(t *testing.T) {
	bins := []*DurationBin{
		{Label: "0-10", Max: 10},
		{Label: "10-20", Max: 20},
		{Label: "20-30", Max: 30},
		{Label: "30-40", Max: 40},
		{Label: "40-50", Max: 50},
	}
	h, err := NewDurationHistogramBins(bins)
	require.NoError(t, err)

	// Uniformly distributed data from 0 to 50
	data := []time.Duration{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
		11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
		21, 22, 23, 24, 25, 26, 27, 28, 29, 30,
		31, 32, 33, 34, 35, 36, 37, 38, 39, 40,
		41, 42, 43, 44, 45, 46, 47, 48, 49, 50,
	}

	for _, d := range data {
		h.Observe(d)
	}

	// This test is a bit brittle to details, don't worry if it breaks with changes as long as the
	// new values are close.
	require.Equal(t, int64(50), h.TotalCount())
	require.Equal(t, time.Duration(10), h.Quantile(0.2))
	require.Equal(t, time.Duration(49), h.Quantile(0.99))
	require.Equal(t, time.Duration(25), h.Quantile(0.5))
	require.Equal(t, time.Duration(45), h.Quantile(0.9))
}

func TestUnboundedQuantileEstimation(t *testing.T) {
	bins := []*DurationBin{
		{Label: "0-10", Max: 10},
		{Label: "10-"},
	}

	h, err := NewDurationHistogramBins(bins)
	require.NoError(t, err)
	h.Observe(5)
	// The 10- bin has an average of 30
	// Therefore we estimate that the bin is evenly distributed across 10-50 (twice the difference
	// between the previous max and the average)
	h.Observe(20)
	h.Observe(30)
	h.Observe(30)
	h.Observe(40)

	// The upper 80% of this histogram is estimated as the linear distribution from 10-50
	require.Equal(t, time.Duration(10), h.Quantile(0.20))
	require.Equal(t, time.Duration(30), h.Quantile(0.60))
	require.Equal(t, time.Duration(50), h.Quantile(1.0))
}

func TestMarshalUnmarshal(t *testing.T) {
	h := NewDurationHistogram(DefaultDurationHistogram)

	data := []time.Duration{
		10 * time.Millisecond,
		10 * time.Millisecond,
		100 * time.Millisecond,
		100 * time.Millisecond,
		300 * time.Millisecond,
		300 * time.Millisecond,
	}

	for _, d := range data {
		h.Observe(d)
	}

	s, err := h.MarshalJSON()
	require.NoError(t, err)

	var vals []*SerializedDurationBin
	json.Unmarshal(s, &vals)
	h2 := NewDurationHistogram(DefaultDurationHistogram)
	err = h2.LoadValues(vals)
	require.NoError(t, err)

	s2, err := h2.MarshalJSON()
	require.NoError(t, err)

	require.Equal(t, string(s), string(s2))
}

func TestHistogramConcurrency(t *testing.T) {
	bins := []*DurationBin{
		{Label: "0-10", Max: 10},
		{Label: "10-20", Max: 20},
		{Label: "20-30", Max: 30},
		{Label: "30-40", Max: 40},
		{Label: "40-50", Max: 50},
	}
	h, err := NewDurationHistogramBins(bins)
	require.NoError(t, err)

	// Uniformly distributed data from 0 to 50
	data := []time.Duration{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
		11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
		21, 22, 23, 24, 25, 26, 27, 28, 29, 30,
		31, 32, 33, 34, 35, 36, 37, 38, 39, 40,
		41, 42, 43, 44, 45, 46, 47, 48, 49, 50,
	}

	wg := sync.WaitGroup{}

	for _, d := range data {
		wg.Add(1)
		go func(d time.Duration) {
			h.Observe(d)
			wg.Done()
		}(d)
	}

	wg.Wait()

	// This test is a bit brittle to details, don't worry if it breaks with changes as long as the
	// new values are close.

	// Summary statistics are a simple way to ensure it looks the way it should
	require.Equal(t, int64(50), h.TotalCount())
	require.Equal(t, int64(1275), h.TotalDSum())
	require.Equal(t, time.Duration(10), h.Quantile(0.2))
	require.Equal(t, time.Duration(49), h.Quantile(0.99))
	require.Equal(t, time.Duration(25), h.Quantile(0.5))
	require.Equal(t, time.Duration(45), h.Quantile(0.9))
}
