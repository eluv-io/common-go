package mpegts

// PcrUnwrapper converts 42-bit PCR values to a monotonic int64 sequence, handling wraparound at MaxPCR.
type PcrUnwrapper struct {
	hasLast  bool
	last     uint64
	current  int64
	previous int64
}

// Unwrap converts the given 42-bit PCR value to a monotonic int64 counter. It returns the previous and current
// unwrapped values. On the first call, previous is set to current-1 (a synthetic value).
func (u *PcrUnwrapper) Unwrap(pcr uint64) (previous, current int64) {
	if !u.hasLast {
		u.hasLast = true
		u.last = pcr
		u.current = int64(pcr)
		u.previous = u.current - 1
		return u.previous, u.current
	}

	// PCR is a 42-bit counter. Detect wraparound by checking whether the signed difference exceeds half the range.
	const halfRange = int64(MaxPCR / 2)
	diff := int64(pcr) - int64(u.last)
	if diff < -halfRange {
		diff += int64(MaxPCR) + 1 // wrapped forward
	} else if diff > halfRange {
		diff -= int64(MaxPCR) + 1 // wrapped backward
	}

	u.previous = u.current
	u.current += diff
	u.last = pcr
	return u.previous, u.current
}

// PcrGapDetector detects PCR jumps larger than a configured threshold using a PcrUnwrapper for wraparound-safe
// comparison.
type PcrGapDetector struct {
	Unwrapper PcrUnwrapper
	Threshold uint64 // max allowed delta in PCR ticks; 0 = no gap detection
}

// Detect unwraps the PCR and returns whether a gap (delta > Threshold) was detected. The first call is never
// considered a gap.
func (d *PcrGapDetector) Detect(pcr uint64) (previous, current int64, gap bool) {
	previous, current = d.Unwrapper.Unwrap(pcr)
	if !d.Unwrapper.hasLast {
		return // first ever call — not a gap
	}
	if d.Threshold > 0 {
		delta := current - previous
		if delta < 0 {
			delta = -delta
		}
		gap = uint64(delta) > d.Threshold
	}
	return
}
