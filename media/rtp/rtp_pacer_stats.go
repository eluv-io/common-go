package rtp

import (
	"sync/atomic"
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/statsutil"
	"github.com/eluv-io/utc-go"
)

// InStats tracks pacer input statistics.
type InStats struct {
	// PushAhead is (targetTime - currentTime) when packet is pushed
	PushAhead statsutil.RawStatistics[time.Duration] `json:"push_ahead"`

	// StartupT0Correction tracks negative T0 drift (T0 moving earlier) observed during the startup/discard phase.
	StartupT0Correction statsutil.RawStatistics[time.Duration] `json:"startup_t0_correction"`

	// NegDrift tracks negative T0 drift (T0 moving earlier) observed during the active phase.
	NegDrift statsutil.RawStatistics[time.Duration] `json:"neg_drift"`

	// NegDriftApplied tracks the actually-applied baseTime corrections for negative drift when AdjustTimeDrift is
	// enabled. When MaxNegDriftCorrection is set, this may be less than NegDrift (the nominal observed drift).
	NegDriftApplied statsutil.RawStatistics[time.Duration] `json:"neg_drift_applied,omitempty"`

	// PosDrift records the mean T0 drift for each period in which the mean exceeded PosDriftThreshold.
	// Recorded regardless of whether AdjustTimeDrift is enabled.
	PosDrift statsutil.RawStatistics[time.Duration] `json:"pos_drift,omitempty"`

	// PosDriftApplied records each positive baseTime correction applied by the positive-drift compensator.
	PosDriftApplied statsutil.RawStatistics[time.Duration] `json:"pos_drift_applied,omitempty"`

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
	Wait       statsutil.RawStatistics[duration.Spec] `json:"wait"`       // wait time stats for last period
	IPD        statsutil.RawStatistics[duration.Spec] `json:"ipd"`        // inter-packet delay stats for last period
	CHD        statsutil.RawStatistics[duration.Spec] `json:"chd"`        // channel delay stats for last period
	Lateness   statsutil.RawStatistics[duration.Spec] `json:"late"`       // lateness stats (targetTs < now+DeliveryMargin) for last period
	SendAhead  statsutil.RawStatistics[duration.Spec] `json:"send_ahead"` // send-ahead stats for last period
	OverSleeps statsutil.RawStatistics[duration.Spec] `json:"oversleeps"` // oversleep stats for last period
	BufFill    statsutil.RawStatistics[int32]         `json:"buf"`        // buffer occupancy stats for last period

	BufferedPackets int32 `json:"buffered"` // snapshot of packets in the buffer at last period boundary
	DelayedPackets  int   `json:"delayed"`  // packets popped from the queue after their nominal sending time
	Sleeps          int   `json:"sleeps"`   // number of ticker ticks consumed while waiting
}

// OutStats is a pure collector for pacer output statistics. At each period boundary, switchPeriod constructs and
// returns an OutStatsPeriod snapshot; OutStats itself holds no exported state.
type OutStats struct {
	// private period collectors
	wait       statsutil.Periodic[duration.Spec] // collector for wait times
	ipd        statsutil.Periodic[duration.Spec] // collector for inter-packet delays
	chd        statsutil.Periodic[duration.Spec] // collector for channel delays
	lateness   statsutil.Periodic[duration.Spec] // collector for lateness (targetTs < now+DeliveryMargin)
	sendAhead  statsutil.Periodic[duration.Spec] // collector for send-ahead (sendAt - now)
	oversleeps statsutil.Periodic[duration.Spec] // collector for oversleeping (actualSleep - expectedSleep)
	bufFill    statsutil.Periodic[int32]         // collector for buffer occupancy

	// per-period counters; reset by switchPeriod
	delayedPackets int
	sleeps         int

	buffered   atomic.Int32 // current count of packets in channel
	lastPacket utc.UTC      // wall clock time when the last packet was popped
}

// newOutStats returns an OutStats whose Periodic fields have ManualSwitch: true so that period transitions are
// driven exclusively by logStats() rather than by packet arrival timing. period should be the configured
// StatsInterval so that Statistics.Duration in each snapshot reflects the nominal period length.
func newOutStats(period duration.Spec) OutStats {
	return OutStats{
		wait:       statsutil.Periodic[duration.Spec]{ManualSwitch: true, Period: period},
		ipd:        statsutil.Periodic[duration.Spec]{ManualSwitch: true, Period: period},
		chd:        statsutil.Periodic[duration.Spec]{ManualSwitch: true, Period: period},
		lateness:   statsutil.Periodic[duration.Spec]{ManualSwitch: true, Period: period},
		sendAhead:  statsutil.Periodic[duration.Spec]{ManualSwitch: true, Period: period},
		oversleeps: statsutil.Periodic[duration.Spec]{ManualSwitch: true, Period: period},
		bufFill:    statsutil.Periodic[int32]{ManualSwitch: true, Period: period},
	}
}

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
		DelayedPackets:  s.delayedPackets,
		Sleeps:          s.sleeps,
	}
	s.delayedPackets = 0
	s.sleeps = 0
	return p
}
