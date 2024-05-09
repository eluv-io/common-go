package histogram

import (
	"encoding/json"
	"math"
	"sync"
	"time"

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

const OutlierLabel = "outliers"

func NewDefaultDurationHistogram() *DurationHistogram {
	return NewDurationHistogram(DefaultDurationHistogram)
}

// NewDurationHistogram creates a new duration histogram with predefined
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
			{Label: "30s-"},
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
			{Label: "4s-10s", Max: time.Second * 10},
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
			{Label: "2h-"},
		}
	}
	h, _ := NewDurationHistogramBins(bins)
	return h
}

func (h *DurationHistogram) LoadValues(values []*SerializedDurationBin) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, v := range values {
		seen := false
		for _, b := range h.bins {
			if v.Label == b.Label {
				b.Count = v.Count
				b.DSum = v.DSum
				seen = true
				break
			}
		}
		if !seen {
			return errors.E("DurationHistogramFromValues", "reason", "mismatched histogram types", "label", v.Label)
		}
	}
	return nil
}

type DurationBin struct {
	Label string
	// Max is the immutable upper-bound of the bin (lower bound defined by previous bin). Setting
	// this to 0 represents that the bin has no upper bound
	Max   time.Duration
	Count int64 // the number of durations falling in this bin
	DSum  int64 // sum of durations of this bin
}

type SerializedDurationBin struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
	DSum  int64  `json:"dsum"`
}

// NewDurationHistogramBins creates a histogram from custom duration bins. The bins must be empty
// (Count and DSum equal to 0), and provided in strictly increasing order of bin maximums.
// Optionally, the final bin may have a max of 0 to represent an unbounded bin.
// By convention, the provided labels are usually PREV_MAX-CUR_MAX, where each is formatted in a
// concise readable format.
// An 'outliers' bin is automatically added if there is not an unbounded bin at the end of the
// histogram. It is ignored for summary statistic purposes.
func NewDurationHistogramBins(bins []*DurationBin) (*DurationHistogram, error) {
	e := errors.Template("NewDurationHistogramBins", errors.K.Invalid)

	if len(bins) == 0 {
		return nil, e("reason", "no bins")
	}

	for i, b := range bins {
		// Unbounded bins can only be the final bin
		if b.Max == 0 && i != len(bins)-1 {
			return nil, e("reason", "unbounded bin not final bin", "label", b.Label, "index", i)
		}

		// The outlier bin can only be the final bin
		if b.Label == OutlierLabel && (i != len(bins)-1 || b.Max != 0) {
			return nil, e("reason", "outlier bin not correct", "label", b.Label, "index", i)
		}

		if b.Max != 0 && i > 0 && b.Max <= bins[i-1].Max {
			return nil, e("reason", "bins not strictly increasing", "bin_label", b.Label,
				"bin_max", b.Max, "prev_label", bins[i-1].Label, "prev_max", bins[i-1].Max)
		}

		if b.Count != 0 || b.DSum != 0 {
			return nil, e("reason", "bin for construction not empty", "label", b.Label)
		}
	}

	if bins[len(bins)-1].Max != 0 {
		bins = append(bins, &DurationBin{Label: OutlierLabel, Max: 0})
	}

	return &DurationHistogram{
		bins: bins,
	}, nil
}

// DurationHistogram uses predefined bins.
type DurationHistogram struct {
	bins          []*DurationBin
	mu            sync.Mutex
	MarshalTotals bool
}

func (h *DurationHistogram) TotalCount() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	tot := int64(0)
	for _, b := range h.bins {
		if b.Label == OutlierLabel {
			continue
		}
		tot += b.Count
	}
	return tot
}

func (h *DurationHistogram) TotalDSum() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	tot := int64(0)
	for _, b := range h.bins {
		if b.Label == OutlierLabel {
			continue
		}
		tot += b.DSum
	}
	return tot
}

// OutlierProportion returns the proportion of histogram observations that fall outside the given
// bins. This returns 0 if the histogram includes an unbounded bin at the upper end.
func (h *DurationHistogram) OutlierProportion() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	_, totalCount := h.loadCounts()

	if h.bins[len(h.bins)-1].Label != OutlierLabel {
		return 0
	}

	outCount := h.bins[len(h.bins)-1].Count
	if outCount == 0 && totalCount == 0 {
		return 0
	}
	return float64(outCount) / (float64(totalCount + outCount))
}

func (h *DurationHistogram) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, b := range h.bins {
		b.Count = 0
		b.DSum = 0
	}
}

func (h *DurationHistogram) Observe(n time.Duration) {
	for i := range h.bins {
		if n <= h.bins[i].Max || (i == len(h.bins)-1 && h.bins[i].Max == 0) {
			h.mu.Lock()
			defer h.mu.Unlock()

			h.bins[i].Count += 1
			h.bins[i].DSum += int64(n)
			return
		}
	}
}

// loadCounts preloads counts for consistent use within a calculation, along with the total
func (h *DurationHistogram) loadCounts() ([]int64, int64) {
	data := make([]int64, len(h.bins))
	tot := int64(0)
	for i := range h.bins {
		if h.bins[i].Label == OutlierLabel {
			continue
		}
		data[i] = h.bins[i].Count
		tot += data[i]
	}

	return data, tot
}

// loadDSums preloads durations for consistent use within a calculation, along with the total
func (h *DurationHistogram) loadDSums() ([]time.Duration, time.Duration) {
	data := make([]time.Duration, len(h.bins))
	tot := time.Duration(0)
	for i := range h.bins {
		if h.bins[i].Label == OutlierLabel {
			continue
		}
		data[i] = time.Duration(h.bins[i].DSum)
		tot += data[i]
	}

	return data, tot
}

func (h *DurationHistogram) Average() time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, totCount := h.loadCounts()
	_, totDur := h.loadDSums()
	if totCount == 0 {
		return 0
	}
	trueAvg := float64(totDur) / float64(totCount)
	return time.Duration(trueAvg)
}

// Quantile returns an approximation of the value at the qth quantile of the histogram, where q is
// in the range [0, 1]. It makes use of the assumption that data within each histogram bin is
// uniformly distributed in order to estimate within bins. Depending on the distribution, this
// assumption may be more or less accurate. For the topmost bin that is unbounded, it assumes the
// intra-bin distribution is uniform over the range [bin_min, 2 * bin_average].
func (h *DurationHistogram) Quantile(q float64) time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()

	if q < 0 || q > 1 {
		return -1
	}
	// Pre-load data to ensure consistency within function
	data, tot := h.loadCounts()

	count := q * float64(tot)

	if count == 0 {
		return 0
	}
	for i := range h.bins {
		if h.bins[i].Label == OutlierLabel {
			continue
		}
		fData := float64(data[i])
		if count <= fData {
			binProportion := 1 - ((fData - count) / fData)
			binStart := time.Duration(0)
			if i > 0 {
				binStart = h.bins[i-1].Max
			}
			var binSpan time.Duration
			if h.bins[i].Max != 0 {
				binSpan = h.bins[i].Max - binStart
			} else {
				binAvg := float64(h.bins[i].DSum) / float64(h.bins[i].Count)
				binSpan = (time.Duration(binAvg) - binStart) * 2
			}
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
	h.mu.Lock()
	defer h.mu.Unlock()

	counts, totCount := h.loadCounts()
	durs, totDur := h.loadDSums()

	if totCount == 0 {
		return 0
	}

	trueAvg := float64(totDur) / float64(totCount)
	binAvgs := []float64{}
	for i := range h.bins {
		if h.bins[i].Label == OutlierLabel {
			continue
		}
		if counts[i] == 0 {
			binAvgs = append(binAvgs, 0)
			continue
		}
		binAvg := float64(durs[i]) / float64(counts[i])
		binAvgs = append(binAvgs, binAvg)
	}

	sumOfSquares := float64(0)
	for i := range h.bins {
		if h.bins[i].Label == OutlierLabel {
			continue
		}
		sumOfSquares += float64(counts[i]) * math.Pow(binAvgs[i]-trueAvg, 2)
	}

	stdDev := math.Pow(sumOfSquares/float64(totCount), 0.5)
	return time.Duration(int64(stdDev))
}

func (h *DurationHistogram) MarshalGeneric() interface{} {
	h.mu.Lock()
	defer h.mu.Unlock()

	totalCount := int64(0)
	totalDur := int64(0)

	v := make([]SerializedDurationBin, len(h.bins))
	for i, b := range h.bins {
		v[i].Label = b.Label
		v[i].Count = b.Count
		v[i].DSum = b.DSum

		totalCount += v[i].Count
		totalDur += v[i].DSum
	}
	if !h.MarshalTotals {
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

func (h *DurationHistogram) MarshalArray() []SerializedDurationBin {
	h.mu.Lock()
	defer h.mu.Unlock()

	v := make([]SerializedDurationBin, len(h.bins))
	for i, b := range h.bins {
		v[i].Label = b.Label
		v[i].Count = b.Count
		v[i].DSum = b.DSum
	}
	return v
}

// String returns a string representation of the histogram.
func (h *DurationHistogram) String() string {
	bb, err := json.Marshal(h)
	if err != nil {
		return jsonutil.MarshallingError("duration_histogram", err)
	}
	return string(bb)
}
