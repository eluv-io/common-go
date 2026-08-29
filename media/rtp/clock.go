package rtp

import "time"

// TicksToDuration converts an RTP timestamp (90 kHz clock) to a time.Duration.
//
// int64 overflow cap: ~292 years with the q/r split below, ~32.5 years without it.
func TicksToDuration(ts int64) time.Duration {
	// RTP with video uses a 90kHz clock, i.e. 1 tick = 1/90000 s or 1s = 90000 ticks
	q, r := ts/9, ts%9
	return time.Duration(q)*100*time.Microsecond + time.Duration(r)*100*time.Microsecond/9
}

// DurationToTicks converts a time.Duration to an RTP timestamp (90 kHz clock).
//
// int64 overflow cap: no overflow with the q/r split below, ~32.5 years without it.
//
// The original implementation was int64((ts*9+1)/100/time.Microsecond) - the "+1" was a rounding hack that, besides
// contributing to the overflow above, was also a genuine off-by-one bug: it rounded up whenever ts*9 mod 100000 ==
// 99999 (one specific nanosecond value in every 100µs), independent of the overflow fix. This version drops it and
// computes the mathematically exact floor(ts*9/100000).
func DurationToTicks(ts time.Duration) int64 {
	// RTP with video uses a 90kHz clock, i.e. 1 tick = 1/90000 s or 1s = 90000 ticks
	x := int64(ts)
	q, r := x/100000, x%100000
	return 9*q + 9*r/100000
}
