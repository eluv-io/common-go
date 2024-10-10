package histogram

import (
	"sync"
	"time"
)

// MovingAverageHistogram keeps multiple duration histograms, and rotates out the oldest histograms
// up to a specified maximum duration. It can return both estimates of statistics over the last
// minute, as well as weighted statistics over the duration, with more recent time periods weighted
// more heavily.
type MovingAverageHistogram struct {
	// newHist is the factory for constructing new duration histograms
	newHist func() *DurationHistogram
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
	histFactory func() *DurationHistogram,
	histCount int,
	durationPerHistogram time.Duration,
) (*MovingAverageHistogram, error) {
	mah := &MovingAverageHistogram{
		newHist:              histFactory,
		durationPerHistogram: durationPerHistogram,
		h:                    make([]*DurationHistogram, histCount),
	}
	// Create the first histogram
	mah.h[0] = histFactory()

	return mah, nil
}

func (m *MovingAverageHistogram) Rotate() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := len(m.h) - 1; i > 0; i-- {
		m.h[i] = m.h[i-1]
	}

	m.h[0] = m.newHist()
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
	count := countToKeep(m.durationPerHistogram, time.Minute)

	m.mu.Lock()
	hists := make([]*DurationHistogram, count)
	copy(hists, m.h)
	m.mu.Unlock()

	agg := m.newHist()
	for _, h := range hists {
		if h == nil {
			// The other histograms will also be nil, so we break
			break
		}
		// Ignore the error as we know that the histograms are of the same type.
		_ = agg.Add(h)
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

func countToKeep(durationPerHistogram, durationToCover time.Duration) int {
	// We calculate this countToKeep in order to at least capture the last minute worth of data,
	// accounting for the possibility of an empty period that just started.
	countToKeep := int(durationToCover/durationPerHistogram) + 1
	// This case occurs with the duration per histogram is not a divisor of a minute.
	if time.Duration(countToKeep)*durationPerHistogram < durationToCover+durationPerHistogram {
		countToKeep++
	}
	return countToKeep
}
