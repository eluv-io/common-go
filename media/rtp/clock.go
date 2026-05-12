package rtp

import "time"

// TicksToDuration converts an RTP timestamp (90 kHz clock) to a time.Duration.
func TicksToDuration(ts int64) time.Duration {
	// RTP with video uses a 90kHz clock, i.e. 1 tick = 1/90000 s or 1s = 90000 ticks
	return time.Duration(ts) * 100 * time.Microsecond / 9
}

// DurationToTicks converts a time.Duration to an RTP timestamp (90 kHz clock).
func DurationToTicks(ts time.Duration) int64 {
	// RTP with video uses a 90kHz clock, i.e. 1 tick = 1/90000 s or 1s = 90000 ticks
	return int64((ts*9 + 1) / 100 / time.Microsecond)
}
