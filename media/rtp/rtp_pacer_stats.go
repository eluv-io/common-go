package rtp

import (
	"sync/atomic"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/statsutil"
	"github.com/eluv-io/utc-go"
)

// InStats tracks pacer input statistics.
type InStats struct {
	// PushAhead is (targetTime - currentTime) when packet is pushed
	PushAhead statsutil.RawStatistics[duration.Millis] `json:"push_ahead"`

	// StartupT0Correction tracks negative T0 drift (T0 moving earlier) observed during the startup/discard phase.
	StartupT0Correction statsutil.RawStatistics[duration.Millis] `json:"startup_t0_correction"`

	// NegDrift tracks negative T0 drift (T0 moving earlier) observed during the active phase.
	NegDrift statsutil.RawStatistics[duration.Millis] `json:"neg_drift"`

	// NegDriftApplied tracks the actually-applied baseTime corrections for negative drift when AdjustTimeDrift is
	// enabled. When MaxNegDriftCorrection is set, this may be less than NegDrift (the nominal observed drift).
	NegDriftApplied statsutil.RawStatistics[duration.Millis] `json:"neg_drift_applied,omitempty"`

	// PosDrift records the mean T0 drift for each period in which the mean exceeded PosDriftThreshold.
	// Recorded regardless of whether AdjustTimeDrift is enabled.
	PosDrift statsutil.RawStatistics[duration.Millis] `json:"pos_drift,omitempty"`

	// PosDriftApplied records each positive baseTime correction applied by the positive-drift compensator.
	PosDriftApplied statsutil.RawStatistics[duration.Millis] `json:"pos_drift_applied,omitempty"`

	// Minimum T0 seen, zero value means not set
	MinT0 utc.UTC `json:"min_t0"`

	// The number of times the stream has been reset due to an RTP gap
	StreamResets int `json:"stream_resets,omitempty"`

	// The time of the last stream reset
	LastStreamReset utc.UTC `json:"last_stream_reset"`
	Seq             uint16  `json:"seq"`  // RTP sequence number of the most recent packet
	Sequ            int64   `json:"sequ"` // unwrapped RTP sequence number of the most recent packet
	Ts              uint32  `json:"ts"`   // RTP timestamp of the most recent packet
	Tsu             int64   `json:"tsu"`  // unwrapped RTP timestamp of the most recent packet
}

// Reset clears all per-session statistics. Lifetime counters (StreamResets, LastStreamReset) are preserved so that
// gap-triggered resets do not lose the accumulated reset history.
func (s *InStats) Reset() {
	streamResets := s.StreamResets
	lastReset := s.LastStreamReset
	*s = InStats{}
	s.StreamResets = streamResets
	s.LastStreamReset = lastReset
}

// OutStatsPeriod holds the per-period output statistics snapshot. It contains only exported Statistics[T] fields
// and plain counters, so it is safe to copy by value (no atomics or sync values). It is used as the snapshot type
// for cross-goroutine stats publishing.
type OutStatsPeriod struct {
	Wait       statsutil.RawStatistics[duration.Millis] `json:"wait"`       // wait time stats for last period
	IPD        statsutil.RawStatistics[duration.Millis] `json:"ipd"`        // inter-packet delay stats for last period
	CHD        statsutil.RawStatistics[duration.Millis] `json:"chd"`        // channel delay stats for last period
	Lateness   statsutil.RawStatistics[duration.Millis] `json:"late"`       // lateness stats (targetTs < now+DeliveryMargin) for last period
	SendAhead  statsutil.RawStatistics[duration.Millis] `json:"send_ahead"` // send-ahead stats for last period
	OverSleeps statsutil.RawStatistics[duration.Millis] `json:"oversleeps"` // oversleep stats for last period
	BufFill    statsutil.RawStatistics[int32]           `json:"buf"`        // buffer occupancy stats for last period

	BufferedPackets int32 `json:"buffered"` // snapshot of packets in the buffer at last period boundary
	Sleeps          int   `json:"sleeps"`   // number of ticker ticks consumed while waiting
}

// OutStats is a pure collector for pacer output statistics. At each period boundary, switchPeriod constructs and
// returns an OutStatsPeriod snapshot; OutStats itself holds no exported state.
type OutStats struct {
	// private period collectors
	wait       statsutil.Periodic[duration.Millis] // collector for wait times
	ipd        statsutil.Periodic[duration.Millis] // collector for inter-packet delays
	chd        statsutil.Periodic[duration.Millis] // collector for channel delays
	lateness   statsutil.Periodic[duration.Millis] // collector for lateness (targetTs < now+DeliveryMargin)
	sendAhead  statsutil.Periodic[duration.Millis] // collector for send-ahead (sendAt - now)
	oversleeps statsutil.Periodic[duration.Millis] // collector for oversleeping (actualSleep - expectedSleep)
	bufFill    statsutil.Periodic[int32]           // collector for buffer occupancy

	// per-period counters; reset by switchPeriod
	sleeps int

	buffered   atomic.Int32 // current count of packets in channel
	lastPacket utc.UTC      // wall clock time when the last packet was popped
}

// NewOutStats returns an OutStats whose Periodic fields have ManualSwitch: true so that period transitions are
// driven exclusively by logStats() rather than by packet arrival timing. period should be the configured
// StatsInterval so that Statistics.Duration in each snapshot reflects the nominal period length.
func NewOutStats(period duration.Spec) OutStats {
	return newOutStats(period)
}

func newOutStats(period duration.Spec) OutStats {
	return OutStats{
		wait:       statsutil.Periodic[duration.Millis]{ManualSwitch: true, Period: period},
		ipd:        statsutil.Periodic[duration.Millis]{ManualSwitch: true, Period: period},
		chd:        statsutil.Periodic[duration.Millis]{ManualSwitch: true, Period: period},
		lateness:   statsutil.Periodic[duration.Millis]{ManualSwitch: true, Period: period},
		sendAhead:  statsutil.Periodic[duration.Millis]{ManualSwitch: true, Period: period},
		oversleeps: statsutil.Periodic[duration.Millis]{ManualSwitch: true, Period: period},
		bufFill:    statsutil.Periodic[int32]{ManualSwitch: true, Period: period},
	}
}

// SwitchPeriod closes the current period, resets per-period counters, and returns the completed period's snapshot.
// Must be called under outStatsMu.
func (s *OutStats) SwitchPeriod(now utc.UTC) *OutStatsPeriod { return s.switchPeriod(now) }

// Total returns a snapshot of cumulative statistics since startup. Must be called under outStatsMu.
func (s *OutStats) Total() *OutStatsPeriod { return s.total() }

// IncrBuffered atomically increments the in-flight packet counter.
func (s *OutStats) IncrBuffered() { s.buffered.Add(1) }

// DecrBuffered atomically decrements the in-flight packet counter and returns the new value.
func (s *OutStats) DecrBuffered() int32 { return s.buffered.Add(-1) }

// UpdateBufFill records the current buffer fill level. Must be called under outStatsMu.
func (s *OutStats) UpdateBufFill(now utc.UTC, fill int32) { s.bufFill.UpdateNow(now, fill) }

// UpdateOversleeps records an oversleep sample. Must be called under outStatsMu.
func (s *OutStats) UpdateOversleeps(now utc.UTC, v duration.Millis) { s.oversleeps.UpdateNow(now, v) }

// UpdateLateness records a lateness sample. Must be called under outStatsMu.
func (s *OutStats) UpdateLateness(now utc.UTC, v duration.Millis) { s.lateness.UpdateNow(now, v) }

// UpdateSendAhead records a send-ahead sample. Must be called under outStatsMu.
func (s *OutStats) UpdateSendAhead(now utc.UTC, v duration.Millis) { s.sendAhead.UpdateNow(now, v) }

// UpdateIPD records an inter-packet delay sample, using the internally tracked lastPacket timestamp.
// Must be called under outStatsMu.
func (s *OutStats) UpdateIPD(now utc.UTC) {
	if s.lastPacket.IsZero() {
		s.ipd.UpdateNow(now, 0)
	} else {
		s.ipd.UpdateNow(now, duration.Millis(now.Sub(s.lastPacket)))
	}
	s.lastPacket = now
}

// UpdateCHD records a channel delay (time from packet arrival to delivery) sample. Must be called under outStatsMu.
func (s *OutStats) UpdateCHD(now utc.UTC, inTs utc.UTC) {
	s.chd.UpdateNow(now, duration.Millis(now.Sub(inTs)))
}

// UpdateWait records a wait-time sample. Must be called under outStatsMu.
func (s *OutStats) UpdateWait(now utc.UTC, v duration.Millis) { s.wait.UpdateNow(now, v) }

// AddSleeps increments the per-period sleep counter. Must be called under outStatsMu.
func (s *OutStats) AddSleeps(n int) { s.sleeps += n }

// switchPeriod closes the current period, resets per-period counters, and returns the completed period's snapshot.
// It must be called under outStatsMu. With ManualSwitch: true on all Periodic fields, every Switch call here is
// explicit — UpdateNow in Handle() never auto-switches.
func (s *OutStats) switchPeriod(now utc.UTC) *OutStatsPeriod {
	s.wait.Switch(now)
	s.ipd.Switch(now)
	s.chd.Switch(now)
	s.lateness.Switch(now)
	s.sendAhead.Switch(now)
	s.oversleeps.Switch(now)
	s.bufFill.Switch(now)

	p := &OutStatsPeriod{
		Wait:            s.wait.Previous.Raw(),
		IPD:             s.ipd.Previous.Raw(),
		CHD:             s.chd.Previous.Raw(),
		Lateness:        s.lateness.Previous.Raw(),
		SendAhead:       s.sendAhead.Previous.Raw(),
		OverSleeps:      s.oversleeps.Previous.Raw(),
		BufFill:         s.bufFill.Previous.Raw(),
		BufferedPackets: s.buffered.Load(),
		Sleeps:          s.sleeps,
	}
	s.sleeps = 0
	return p
}

// total returns a snapshot of the total statistics since startup. It must be called under outStatsMu. BufferedPackets
// and Sleeps will be uninitialized.
func (s *OutStats) total() *OutStatsPeriod {
	p := &OutStatsPeriod{
		Wait:       s.wait.Total.Raw(),
		IPD:        s.ipd.Total.Raw(),
		CHD:        s.chd.Total.Raw(),
		Lateness:   s.lateness.Total.Raw(),
		SendAhead:  s.sendAhead.Total.Raw(),
		OverSleeps: s.oversleeps.Total.Raw(),
		BufFill:    s.bufFill.Total.Raw(),
	}
	return p
}
