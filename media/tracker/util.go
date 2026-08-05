package tracker

import (
	"math"
	"time"

	"github.com/eluv-io/common-go/util/statsutil"
)

// rates computes the packet and bit rate for the given counts over the given duration.
func rates(packets, bytes uint64, dur time.Duration) (pps, bps float64) {
	secs := dur.Seconds()
	if secs <= 0 {
		return 0, 0
	}
	return float64(packets) / secs, float64(bytes) * 8 / secs
}

// stddev returns the (sample) standard deviation of the collected statistics, or 0 if it is undefined.
func stddev[T statsutil.Number](s *statsutil.Statistics[T]) float64 {
	s.CalcVariance(true)
	if s.Variance <= 0 {
		return 0
	}
	return math.Sqrt(s.Variance)
}

// round rounds f to the given number of decimal places, mapping NaN and infinities to 0.
func round(f float64, decimals int) float64 {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	p := math.Pow(10, float64(decimals))
	return math.Round(f*p) / p
}
