package histogram

import (
	"encoding/json"
	"math"
	"strings"
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

func TestDurationHistogram(t *testing.T) {
	h := NewDefaultDurationHistogram()

	for i, b := range h.bins {
		min := time.Duration(0)
		if i > 0 {
			min = h.bins[i-1].Max
		}
		h.Observe(min)
		h.Observe(b.Max)
		h.Observe(b.Max + 1)
	}
	for i, b := range h.bins {
		require.Equal(t, int64(3), b.Count.Load())
		switch i {
		case 0:
			require.EqualValues(t, b.Max*2, b.DSum.Load())
		case len(h.bins) - 1:
			require.EqualValues(t, b.Max*2+h.bins[i-1].Max+2, b.DSum.Load())
		default:
			require.EqualValues(t, b.Max*2+h.bins[i-1].Max+1, b.DSum.Load())
		}
	}
}

func TestStandardDeviation(t *testing.T) {
	bins := []*DurationBin{
		{Label: "0-10", Max: 10},
		{Label: "10-50", Max: 50},
		{Label: "50-90", Max: 90},
		{Label: "90-100", Max: 100},
	}
	h := NewDurationHistogramBins(bins)

	data := []time.Duration{
		3, 7, 12, 13, 14, 15, 15, 15, 16, 17, 25, 30, 60, 70, 80, 85, 93,
	}

	for _, d := range data {
		h.Observe(d)
	}

	require.Equal(t, time.Duration(29), h.StandardDeviation())
}

func TestQuantileEstimation(t *testing.T) {
	bins := []*DurationBin{
		{Label: "0-10", Max: 10},
		{Label: "10-20", Max: 20},
		{Label: "20-30", Max: 30},
		{Label: "30-40", Max: 40},
		{Label: "40-50", Max: 50},
	}
	h := NewDurationHistogramBins(bins)

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
