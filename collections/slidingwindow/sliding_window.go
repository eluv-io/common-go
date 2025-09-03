package slidingwindow

import (
	"math"
	"slices"

	"golang.org/x/exp/constraints"

	"github.com/eluv-io/common-go/util/ifutil"
	"github.com/eluv-io/utc-go"
)

type Number interface {
	constraints.Integer | constraints.Float
}

func New[T Number](capacity int) *SlidingWindow[T] {
	if capacity <= 0 {
		capacity = 1
	}
	return &SlidingWindow[T]{
		capacity: capacity,
		entries:  make([]*entry[T], capacity),
		oldest:   0,
		count:    0,
	}
}

// SlidingWindow is a data structure that maintains a fixed-capacity sliding window of the last N values added to it.
// It maintains the mean and variance of the values that are added (and removed) progressively using [Welford's
// algorithm].
//
// Values are stored in a circular buffer and timestamped at insertion. The Statistics method can extract statistical
// summaries (mean, variance, min, max, quantiles, etc.) for all or a subset of values, optionally filtered by a start
// time.
//
// [Welford's algorithm]: https://en.wikipedia.org/wiki/Algorithms_for_calculating_variance#Welford%27s_online_algorithm
type SlidingWindow[T Number] struct {
	entries           []*entry[T] // the values in the series, stored in a circular buffer
	capacity          int         // the maximum number of values in the series
	oldest            int         // index of the oldest value
	count             int         // number of values in the series
	mean              float64     // mean of values in the series
	m2                float64     // sum of squares of differences from the current mean
	useSampleVariance bool        // whether to use sample variance (N-1) or population variance (N)
}

// UseSampleVariance sets the variance calculation to use sample variance (N-1) instead of population variance (N).
// By default, sample variance is used.
func (s *SlidingWindow[T]) UseSampleVariance() *SlidingWindow[T] {
	s.useSampleVariance = true
	return s
}

// UsePopulationVariance sets the variance calculation to use population variance (N) instead of sample variance (N-1).
// By default, sample variance is used.
func (s *SlidingWindow[T]) UsePopulationVariance() *SlidingWindow[T] {
	s.useSampleVariance = false
	return s
}

// Add adds a new value to the sliding window. If the window is full, it replaces the oldest value with the new one.
func (s *SlidingWindow[T]) Add(value T) {
	newValue := float64(value)

	if s.count < s.capacity {
		s.entries[s.count] = &entry[T]{value: value, ts: utc.Now()}
		s.count++

		// Update mean and m2 using Welford's method
		if s.count == 1 {
			s.mean = newValue
			s.m2 = 0.0
		} else {
			delta := newValue - s.mean
			s.mean += delta / float64(s.count)
			s.m2 += delta * (newValue - s.mean)
		}
	} else {
		oldestEntry := s.entries[s.oldest]
		oldValue := float64(oldestEntry.value)

		// replace the oldest value with the new one
		oldestEntry.value = value
		oldestEntry.ts = utc.Now()
		s.oldest = (s.oldest + 1) % s.capacity

		// Update mean and m2 for both removal and addition
		deltaRemove := oldValue - s.mean
		s.mean -= deltaRemove / float64(s.capacity-1)
		s.m2 -= deltaRemove * (oldValue - s.mean)

		deltaAdd := newValue - s.mean
		s.mean += deltaAdd / float64(s.capacity)
		s.m2 += deltaAdd * (newValue - s.mean)
	}

}

// Count returns the number of values currently in the sliding window.
func (s *SlidingWindow[T]) Count() int {
	return s.count
}

// Mean returns the current mean of the values in the sliding window.
func (s *SlidingWindow[T]) Mean() float64 {
	return s.mean
}

// Variance returns the current variance of the values in the sliding window.
func (s *SlidingWindow[T]) Variance() float64 {
	return variance(s.useSampleVariance, s.m2, s.count)
}

// Stddev returns the standard deviation of the values in the sliding window (square root of the variance).
func (s *SlidingWindow[T]) Stddev() float64 {
	return math.Sqrt(s.Variance())
}

// Statistics returns basic statistics of the values in the sliding window, optionally filtered by the specified start
// time.
func (s *SlidingWindow[T]) Statistics(startingAt ...utc.UTC) *Statistics[T] {
	if s.count == 0 {
		return &Statistics[T]{}
	}

	start := ifutil.FirstOrDefault(startingAt, utc.Zero)
	var sum T
	var sum2 float64
	subset := make([]T, 0, s.count)

	for i := 0; i < s.count; i++ {
		e := s.entries[i]
		if e.ts.Before(start) {
			continue // skip entries before the starting time
		}
		value := e.value
		subset = append(subset, value)
		sum += value
		diff := float64(value) - s.mean
		sum2 += diff * diff
	}

	count := len(subset)
	if count == 0 {
		return &Statistics[T]{}
	}

	meanSubset := float64(sum) / float64(count)
	if count != s.count {
		// we were using the wrong mean (s.mean) in the variance calculation above, so we need to recalculate it with
		// meanSubset...
		sum2 = 0.0
		for _, value := range subset {
			diff := float64(value) - meanSubset
			sum2 += diff * diff
		}
	}

	slices.Sort(subset)
	return &Statistics[T]{
		sorted:   subset,
		mean:     meanSubset,
		min:      subset[0],
		max:      subset[count-1],
		variance: variance(s.useSampleVariance, sum2, count),
	}
}

type entry[T any] struct {
	value T       // the value of the entry
	ts    utc.UTC // the time of entry assignment
}

// Statistics provides statistical summaries for a set of numeric values in a SlidingWindow.
// It includes the sorted values, mean, variance, minimum, maximum, and quantile calculations.
type Statistics[T Number] struct {
	sorted   []T // the values in the series
	mean     float64
	variance float64
	min      T
	max      T
}

// Min returns the minimum value in the series.
func (s *Statistics[T]) Min() T {
	return s.min
}

// Max returns the maximum value in the series.
func (s *Statistics[T]) Max() T {
	return s.max
}

// Mean returns the mean (average) of the values in the series.
func (s *Statistics[T]) Mean() float64 {
	return s.mean
}

// Variance returns the variance of the values in the series.
func (s *Statistics[T]) Variance() float64 {
	return s.variance
}

// Stddev returns the standard deviation of the values in the series.
func (s *Statistics[T]) Stddev() float64 {
	return math.Sqrt(s.variance)
}

// Quantile returns the value at the specified quantile (0.0 to 1.0) in the sorted series using the "nearest rank"
// method.
func (s *Statistics[T]) Quantile(q float64) T {
	var zero T
	if q < 0 || q > 1 {
		return zero
	}
	count := len(s.sorted)
	if count == 0 {
		return zero
	}

	// Calculate the index in the sorted order
	index := int(math.Ceil(q * float64(count-1)))
	if index >= count {
		index = count - 1
	}

	return s.sorted[index]
}

// QuantileInterpolated returns the value at the specified quantile (0.0 to 1.0) in the sorted series using linear
// interpolation.
func (s *Statistics[T]) QuantileInterpolated(q float64) T {
	if q < 0 || q > 1 {
		return 0
	}
	count := len(s.sorted)
	if count == 0 {
		return 0
	}

	pos := q * float64(count-1)
	v1 := s.sorted[int(math.Floor(pos))]
	v2 := s.sorted[int(math.Ceil(pos))]
	return T(float64(v1) + (float64(v2)-float64(v1))*(pos-math.Floor(pos)))
}

// Count returns the number of values in the series.
func (s *Statistics[T]) Count() int {
	return len(s.sorted)
}

// variance calculates the variance from the sum of squares of differences (m2) and the count of values. If
// `useSampleVariance` is true, it uses sample variance (N-1) instead of population variance (N).
//
// See https://www.geeksforgeeks.org/maths/sample-variance-vs-population-variance/
func variance(useSampleVariance bool, m2 float64, count int) float64 {
	if useSampleVariance {
		count--
	}
	if count <= 0 {
		return 0.0
	}
	return m2 / float64(count)
}
