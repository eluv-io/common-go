package rtp

import (
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/errors-go"
)

// NewRtpGapDetector creates a new RTP gap detector that checks consecutive RTP packets for unexpected gaps in sequence
// numbers and timestamps. The detection triggers when the difference between the current and previous sequence number
// or timestamp is greater than their respective thresholds.
func NewRtpGapDetector(sequenceThreshold int64, timestampThreshold time.Duration) *GapDetector {
	return &GapDetector{
		SequenceThreshold:  sequenceThreshold,
		TimestampThreshold: DurationToTicks(timestampThreshold),
	}
}

// GapDetector detects RTP stream resets.
type GapDetector struct {
	Sequence           SequenceUnwrapper
	Timestamp          TimestampUnwrapper
	SequenceThreshold  int64
	TimestampThreshold int64
}

// Detect returns the current unwrapped sequence number and timestamp, and a non-nil error if a gap is detected. A gap
// is signaled if the difference between the new and previous sequence numbers or timestamps is greater than their
// respective thresholds.
func (r *GapDetector) Detect(seq uint16, ts uint32) (seqUnwrapped, tsUnwrapped int64, err error) {
	{
		previous, current := r.Sequence.Unwrap(seq)
		diff := current - previous
		if r.abs(diff) > r.SequenceThreshold {
			err = errors.Append(
				err,
				errors.NoTrace("rtp gap detection", errors.K.Invalid,
					"reason", "sequence number gap",
					"previous", previous,
					"current", current,
					"diff", diff,
					"threshold", r.SequenceThreshold,
				),
			)
		}
		seqUnwrapped = current
	}
	{
		previous, current := r.Timestamp.Unwrap(ts)
		diff := current - previous
		if r.abs(diff) > r.TimestampThreshold {
			err = errors.Append(
				err,
				errors.NoTrace("rtp gap detection", errors.K.Invalid,
					"reason", "timestamp gap",
					"previous", previous,
					"current", current,
					"diff", diff,
					"dur", duration.Spec(TicksToDuration(diff)).Round(),
					"threshold", r.TimestampThreshold,
				),
			)
		}
		tsUnwrapped = current
	}
	return
}

func (r *GapDetector) abs(num int64) int64 {
	if num < 0 {
		return -num
	}
	return num
}
