package histogram

import (
	"encoding/json"
	"math"
	"time"

	"go.uber.org/atomic"

	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/errors-go"
)

// ----- durationHistogram -----
type DurationHistogramType uint8

const (
	DefaultDurationHistogram = iota
	ConnectionResponseDurationHistogram
	SegmentLatencyHistogram
)

func NewDefaultDurationHistogram() *DurationHistogram {
	return NewDurationHistogram(DefaultDurationHistogram)
}

// newDurationHistogram creates a new duration histogram with predefined
// labeled duration bins.
// note: label are provided since computing them produces string with useless
// suffixes like: 1m => 1m0s
func NewDurationHistogram(t DurationHistogramType) *DurationHistogram {
	var bins []*DurationBin
	switch t {
	case ConnectionResponseDurationHistogram:
		bins = []*DurationBin{
			{Label: "0-10ms", Max: time.Millisecond * 10},
			{Label: "10ms-20ms", Max: time.Millisecond * 20},
			{Label: "20ms-50ms", Max: time.Millisecond * 50},
			{Label: "50ms-100ms", Max: time.Millisecond * 100},
			{Label: "100ms-200ms", Max: time.Millisecond * 200},
			{Label: "200ms-500ms", Max: time.Millisecond * 500},
			{Label: "500ms-1s", Max: time.Second},
			{Label: "1s-2s", Max: time.Second * 2},
			{Label: "2s-5s", Max: time.Second * 5},
			{Label: "5s-10s", Max: time.Second * 10},
			{Label: "10s-20s", Max: time.Second * 20},
			{Label: "20s-30s", Max: time.Second * 30},
			{Label: "30s-", Max: time.Hour * 10000},
		}
	case SegmentLatencyHistogram:
		bins = []*DurationBin{
			{Label: "0-100ms", Max: time.Millisecond * 100},
			{Label: "100ms-200ms", Max: time.Millisecond * 200},
			{Label: "200ms-300ms", Max: time.Millisecond * 300},
			{Label: "300ms-400ms", Max: time.Millisecond * 400},
			{Label: "400ms-500ms", Max: time.Millisecond * 500},
			{Label: "500ms-750ms", Max: time.Millisecond * 750},
			{Label: "750ms-1s", Max: time.Second},
			{Label: "1s-2s", Max: time.Second * 2},
			{Label: "2s-4s", Max: time.Second * 4},
			{Label: "4s-", Max: time.Second * 30},
		}
	case DefaultDurationHistogram:
		fallthrough
	default:
		bins = []*DurationBin{
			{Label: "0-10ms", Max: time.Millisecond * 10},
			{Label: "10ms-20ms", Max: time.Millisecond * 20},
			{Label: "20ms-50ms", Max: time.Millisecond * 50},
			{Label: "50ms-100ms", Max: time.Millisecond * 100},
			{Label: "100ms-200ms", Max: time.Millisecond * 200},
			{Label: "200ms-500ms", Max: time.Millisecond * 500},
			{Label: "500ms-1s", Max: time.Second},
			{Label: "1s-2s", Max: time.Second * 2},
			{Label: "2s-5s", Max: time.Second * 5},
			{Label: "5s-10s", Max: time.Second * 10},
			{Label: "10s-20s", Max: time.Second * 20},
			{Label: "20s-30s", Max: time.Second * 30},
			{Label: "30s-1m", Max: time.Minute},
			{Label: "1m-2m", Max: time.Minute * 2},
			{Label: "2m-5m", Max: time.Minute * 5},
			{Label: "5m-10m", Max: time.Minute * 10},
			{Label: "10m-20m", Max: time.Minute * 20},
			{Label: "20m-30m", Max: time.Minute * 30},
			{Label: "30m-2h", Max: time.Hour * 2},
			{Label: "2h-", Max: time.Hour * 10000},
		}
	}
	return newDurationHistogramBins(bins)
}

func DurationHistogramFromValues(t DurationHistogramType, values []*SerializedDurationBin) (*DurationHistogram, error) {
	h := NewDurationHistogram(t)
	for _, v := range values {
		seen := false
		for _, b := range h.bins {
			if v.Label == b.Label {
				b.Count.Store(v.Count)
				b.DSum.Store(v.DSum)
				seen = true
				break
			}
		}
		if !seen {
			return nil, errors.E("DurationHistogramFromValues", "reason", "mismatched histogram types", "label", v.Label)
		}
	}
	return h, nil
}

type DurationBin struct {
	Label string
	Max   time.Duration // immutable upper-bound of bin (lower bound defined by previous bin)
	Count atomic.Int64  // number or durations falling in bin
	DSum  atomic.Int64  // sum of durations of this bin
}

type SerializedDurationBin struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
	DSum  int64  `json:"dsum"`
}

func newDurationHistogramBins(bins []*DurationBin) *DurationHistogram {
	return &DurationHistogram{
		bins: bins,
	}
}

// DurationHistogram uses predefined bins.
type DurationHistogram struct {
	bins          []*DurationBin
	marshalTotals bool
}

func (h *DurationHistogram) TotalCount() int64 {
	tot := int64(0)
	for i := range h.bins {
		tot += h.bins[i].Count.Load()
	}
	return tot
}

func (h *DurationHistogram) TotalDSum() int64 {
	tot := int64(0)
	for i := range h.bins {
		tot += h.bins[i].DSum.Load()
	}
	return tot
}

func (h *DurationHistogram) Observe(n time.Duration) {
	for i := range h.bins {
		if n <= h.bins[i].Max || i == len(h.bins)-1 {
			h.bins[i].Count.Add(1)
			h.bins[i].DSum.Add(int64(n))
			return
		}
	}
}

// loadCounts preloads counts for consistent use within a calculation, along with the total
func (h *DurationHistogram) loadCounts() ([]int64, int64) {
	data := make([]int64, len(h.bins))
	tot := int64(0)
	for i := range h.bins {
		data[i] = h.bins[i].Count.Load()
		tot += data[i]
	}

	return data, tot
}

// loadDSums preloads durations for consistent use within a calculation, along with the total
func (h *DurationHistogram) loadDSums() ([]time.Duration, time.Duration) {
	data := make([]time.Duration, len(h.bins))
	tot := time.Duration(0)
	for i := range h.bins {
		data[i] = time.Duration(h.bins[i].DSum.Load())
		tot += data[i]
	}

	return data, tot
}

// Quantile returns an approximation of the value at the qth quantile of the histogram, where q is
// in the range [0, 1]. It makes use of the assumption that data within each histogram bin is
// uniformly distributed in order to estimate within bins. Depending on the distribution, this
// assumption may be more or less accurate.
func (h *DurationHistogram) Quantile(q float64) time.Duration {
	if q < 0 || q > 1 {
		return -1
	}
	// Pre-load data to ensure consistency within function
	data, tot := h.loadCounts()

	count := q * float64(tot)
	for i := range h.bins {
		fData := float64(data[i])
		if count <= fData {
			binProportion := 1 - ((fData - count) / fData)
			binStart := time.Duration(0)
			if i > 0 {
				binStart = h.bins[i-1].Max
			}
			binSpan := h.bins[i].Max - binStart
			return binStart + time.Duration(int64(binProportion*float64(binSpan)))
		}
		count -= fData
	}
	return -1
}

// StandardDeviation estimates the standard deviation using the average of each histogram box. It
// will consistently slightly underestimate the standard deviation as a consequence, because
// variation within each box is not captured in the standard deviation.
//
// A worked example is below to provide intuition:
//
// Our histogram bins are `[[0, 10], [10, 50], [50, 90], [90, 100]]`.  Our data is `[3, 7, 12, 13,
// 14, 15, 15, 15, 16, 17, 25, 30, 60, 70, 80, 85, 93]`.
//
// The true standard deviation of that data is 30.6. The data is concentrated on a peak at 15, and
// has another more spread out area around 80. Calculating the standard deviation with just the bin
// endpoints or midpoints would be quite inaccurate, because the bins are not close to uniformly
// filled with values.
//
// The bin averages are `[5, 17, 74, 93]`, with counts `[2, 10, 4, 1]`. We thus use the formula for
// standard deviation, assuming that 2 points are at 5, 10 points are at 17, 4 points are at 74, and
// 1 point at 93. However, because we have the true average of all points, we can use that as well.
//
// We then calculate an estimated standard deviation of 29, which is very close to the actual
// standard deviation of 30.6.
func (h *DurationHistogram) StandardDeviation() time.Duration {
	counts, totCount := h.loadCounts()
	durs, totDur := h.loadDSums()
	trueAvg := float64(totDur) / float64(totCount)
	binAvgs := []float64{}
	for i := range h.bins {
		binAvg := float64(durs[i]) / float64(counts[i])
		binAvgs = append(binAvgs, binAvg)
	}

	sumOfSquares := float64(0)
	for i := range h.bins {
		sumOfSquares += float64(counts[i]) * math.Pow(binAvgs[i]-trueAvg, 2)
	}

	stdDev := math.Pow(sumOfSquares/float64(totCount), 0.5)
	return time.Duration(int64(stdDev))
}

func (h *DurationHistogram) MarshalGeneric() interface{} {
	totalCount := int64(0)
	totalDur := int64(0)

	v := make([]SerializedDurationBin, len(h.bins))
	for i, b := range h.bins {
		v[i].Label = b.Label
		v[i].Count = b.Count.Load()
		v[i].DSum = b.DSum.Load()

		totalCount += v[i].Count
		totalDur += v[i].DSum
	}
	if !h.marshalTotals {
		return v
	}

	return map[string]any{
		"count": totalCount,
		"dsum":  totalDur,
		"hist":  v,
	}
}

func (h *DurationHistogram) MarshalJSON() ([]byte, error) {
	return json.Marshal(h.MarshalGeneric())
}

// String returns a string representation of the histogram.
func (h *DurationHistogram) String() string {
	bb, err := json.Marshal(h)
	if err != nil {
		return jsonutil.MarshallingError("duration_histogram", err)
	}
	return string(bb)
}
