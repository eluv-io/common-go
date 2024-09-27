package histogram

import (
	"sync"
	"time"

	"github.com/eluv-io/errors-go"
)

// MovingAverageHistogram keeps multiple duration histograms, and rotates out the oldest histograms
// up to a specified maximum duration. It can return both estimates of statistics over the last
// minute, as well as weighted statistics over the duration, with more recent time periods weighted
// more heavily.
type MovingAverageHistogram struct {
	// typ is the type of duration histogram for this moving average histogram. If necessary, this
	// can be extended to a description of the bins directly.
	typ DurationHistogramType
	// maxDuration is the total history that is kept in the moving average histogram.
	maxDuration time.Duration
	// durationPerHistogram describes the rotation schedule of the histogram.
	durationPerHistogram time.Duration

	// mu is _only_ used to protect rotation of the histograms. Once a reference to a histogram is
	// held, mu can be released.
	mu sync.Mutex
	// h keeps the histograms. h[0] is the active histogram, and histograms get older as they are
	// farther in the slice.
	h []*DurationHistogram

	// doneCh is closed to stop automatically rotating
	doneCh chan struct{}
}

func NewMovingAverageHistogram(
	typ DurationHistogramType,
	maxDuration time.Duration,
	durationPerHistogram time.Duration,
) (*MovingAverageHistogram, error) {
	histCount := maxDuration / durationPerHistogram
	if maxDuration%durationPerHistogram != 0 {
		return nil, errors.E("NewMovingAverageHistogram", errors.K.Invalid, "reason", "maxDuration must be a multiple of durationPerHistogram")
	}

	mah := &MovingAverageHistogram{
		typ:                  typ,
		maxDuration:          maxDuration,
		durationPerHistogram: durationPerHistogram,
		h:                    make([]*DurationHistogram, histCount),
	}
	// Create the first histogram
	mah.h[0] = NewDurationHistogram(typ)

	return mah, nil
}

func (m *MovingAverageHistogram) Rotate() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := len(m.h) - 1; i > 0; i-- {
		m.h[i] = m.h[i-1]
	}

	m.h[0] = NewDurationHistogram(m.typ)
}

func (m *MovingAverageHistogram) autoRotate(d time.Duration) {
	t := time.NewTicker(d)

	for {
		select {
		case <-t.C:
			m.Rotate()
		case <-m.doneCh:
			return
		}
	}
}

func (m *MovingAverageHistogram) Start() {
	go m.autoRotate(m.durationPerHistogram)
}

func (m *MovingAverageHistogram) Stop() {
	close(m.doneCh)
}

func (m *MovingAverageHistogram) Observe(n time.Duration) {
	m.mu.Lock()
	h := m.h[0]
	m.mu.Unlock()

	h.Observe(n)
}

func (m *MovingAverageHistogram) StatLastMinute(f func(h *DurationHistogram) time.Duration) time.Duration {
	// We calculate this countToKeep in order to at least capture the last minute worth of data.
	countToKeep := int(time.Minute / m.durationPerHistogram)
	if time.Minute%m.durationPerHistogram != 0 {
		countToKeep++
	}
	if countToKeep == 1 {
		countToKeep++
	}

	m.mu.Lock()
	hists := make([]*DurationHistogram, countToKeep)
	copy(hists, m.h)
	m.mu.Unlock()

	agg := NewDurationHistogram(m.typ)
	for _, h := range hists {
		// Ignore the error as we know that the histograms are of the same type.
		agg.Add(h)
	}

	return f(agg)
}

// StatWeighted returns a weighted average of a certain statistic over the duration of the moving
//
// The weight is calculated as follows:
//
// - 40% from first quarter of total time
// - 30% from second quarter of total time
// - 20% from third quarter of total time
// - 10% from last quarter of total time
//
// If the histogram is not yet full, the weight is normalized by the total weight so far.
func (m *MovingAverageHistogram) StatWeighted(f func(h *DurationHistogram) time.Duration) time.Duration {
	m.mu.Lock()
	hists := make([]*DurationHistogram, len(m.h))
	copy(hists, m.h)
	m.mu.Unlock()

	totWeight := 0
	statValue := time.Duration(0)
	for i, h := range hists {
		// If we have nil histograms, we have not yet filled the moving average histogram.
		// Thus, we need to normalize by the total weight so far.
		if h == nil {
			break
		}

		quartileIdx := (4 * i) / len(hists)
		weightPer := 4 - quartileIdx
		totWeight += weightPer
		statValue += f(h) * time.Duration(weightPer)
	}

	return statValue / time.Duration(totWeight)
}
