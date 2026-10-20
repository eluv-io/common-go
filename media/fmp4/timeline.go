package fmp4

import (
	"sync"
	"time"
)

// Timeline maps the period-relative sample timestamps of one or more tracks onto a single continuous time axis.
//
// The tracks of one presentation (a video rung and an audio stream) share a period origin, so a single offset per
// epoch keeps them aligned. A new epoch starts explicitly (NewEpoch, e.g. when the source switches to a new recording
// period or a failover hop) or implicitly when a track's time jumps backwards or forward by more than the jump
// threshold. The offset of a new epoch is chosen lazily, from the first sample of either track after the switch, so
// that it lands one frame gap after the last time handed out on the old epoch and the output never moves backwards.
//
// Tick to duration conversion is exact integer arithmetic. A float conversion drifts audibly over hours with the
// 48000 Hz audio and 90000 Hz video timescales in use.
type Timeline struct {
	mu sync.Mutex

	epoch     int
	anchored  bool          // the current epoch's offset has been fixed
	offset    time.Duration // added to every mapped time of the current epoch
	lastOut   map[Kind]time.Duration
	anyOut    bool
	gap       time.Duration // spacing inserted between epochs
	jump      time.Duration // an input discontinuity larger than this starts a new epoch
	autoJumps int
}

// TimelineOption configures a Timeline.
type TimelineOption func(*Timeline)

// WithEpochGap sets the spacing inserted between the last time of an epoch and the first time of the next.
func WithEpochGap(gap time.Duration) TimelineOption {
	return func(t *Timeline) { t.gap = gap }
}

// WithJumpThreshold sets the input discontinuity that starts a new epoch implicitly. 0 disables the detection.
func WithJumpThreshold(jump time.Duration) TimelineOption {
	return func(t *Timeline) { t.jump = jump }
}

// NewTimeline creates a Timeline. By default epochs are spaced by one 30 fps frame and a forward jump of more than
// five seconds or any backward jump of more than one frame starts a new epoch.
func NewTimeline(opts ...TimelineOption) *Timeline {
	t := &Timeline{
		lastOut: make(map[Kind]time.Duration),
		gap:     time.Second / 30,
		jump:    5 * time.Second,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// TicksToDuration converts ticks in the given timescale to a duration without floating point rounding.
func TicksToDuration(ticks int64, timescale uint32) time.Duration {
	if timescale == 0 {
		return 0
	}
	ts := int64(timescale)
	secs := ticks / ts
	rem := ticks % ts
	return time.Duration(secs)*time.Second + time.Duration(rem*int64(time.Second)/ts)
}

// Map converts a sample's DTS and PTS (ticks in the track's timescale) to the continuous time axis.
func (t *Timeline) Map(track *TrackInfo, dts, pts int64) (outDTS, outPTS time.Duration) {
	inDTS := TicksToDuration(dts, track.Timescale)
	inPTS := TicksToDuration(pts, track.Timescale)

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.anchored && t.jump > 0 {
		if last, ok := t.lastOut[track.Kind]; ok {
			candidate := inDTS + t.offset
			if candidate < last-t.gap || candidate > last+t.jump {
				log.Info("timeline discontinuity - starting new epoch",
					"kind", track.Kind, "last", last, "candidate", candidate, "epoch", t.epoch+1)
				t.autoJumps++
				t.newEpochLocked()
			}
		}
	}
	if !t.anchored {
		t.offset = 0
		if t.anyOut {
			latest := time.Duration(0)
			for _, v := range t.lastOut {
				if v > latest {
					latest = v
				}
			}
			t.offset = latest + t.gap - inDTS
		}
		t.anchored = true
	}

	outDTS = inDTS + t.offset
	outPTS = inPTS + t.offset
	t.lastOut[track.Kind] = outDTS
	t.anyOut = true
	return outDTS, outPTS
}

// NewEpoch announces a discontinuity in the input timeline: the next mapped sample of either track fixes the offset
// of the new epoch so it continues after the last time handed out.
func (t *Timeline) NewEpoch() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.newEpochLocked()
}

func (t *Timeline) newEpochLocked() {
	t.epoch++
	t.anchored = false
	// The per-kind last output times stay: they define where the new epoch starts. A kind that was silent across
	// the switch simply continues from the shared offset.
}

// Epoch returns the current epoch number, starting at 0.
func (t *Timeline) Epoch() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.epoch
}

// AutoJumps returns how many epochs were started implicitly by an input discontinuity.
func (t *Timeline) AutoJumps() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.autoJumps
}
