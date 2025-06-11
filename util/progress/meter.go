package progress

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/metrics"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/ifutil"
	"github.com/eluv-io/common-go/util/stringutil"
	"github.com/eluv-io/common-go/util/timeutil"
	"github.com/eluv-io/utc-go"
)

// Meter defines a progress meter - a utility for tracking the rate and progress of events over time. It provides
// methods to record events, calculate rates, and estimate the time to reach the goal. The goal can be updated over time
// as needed.
//
// The Meter is designed to be used with a Ticker, which provides periodic notifications that update the meter's rates.
// The ticker is expected to trigger the Tick() method every 5 seconds. Otherwise the calculated rates may not be
// accurate.
//
// The meter is thread-safe and can be used in concurrent environments.
type Meter interface {
	// Mark records the occurrence of `n` events, incrementing the event count.
	Mark(int64)

	// UpdateGoal updates the targeted number of events.
	UpdateGoal(newGoal int64)

	// Stop stops the meter, preventing further updates or event recording.
	Stop()

	// Goal returns the targeted number of events for this meter to reach.
	Goal() int64

	// Started returns the time when the meter started.
	Started() utc.UTC

	// Count returns the number of events recorded.
	Count() int64

	// Rate returns the one-minute moving average rate of events per second.
	Rate() float64

	// RateMean returns the mean rate of events per second since the meter started.
	RateMean() float64

	// Snapshot creates a read-only snapshot of the meter's current state.
	Snapshot() Meter

	// ETC calculates and returns the estimated time of completion (as a UTC timestamp)
	// and the estimated time remaining (as a duration) to reach the goal based on the current rate.
	ETC() (utc.UTC, time.Duration)

	// Fields returns a slice of key-value pairs representing the meter's state, useful for logging or debugging.
	Fields() []any

	// String returns a formatted string representation of the meter's state for easy readability.
	String() string
}

// NewMeter creates a new progress meter with the specified initial goal and optional ticker. Note that the ticker
// is expected to trigger the Tick() method every 5 seconds - otherwise the calculated rates will not be
// correct.
func NewMeter(goal int64, provider ...timeutil.Ticker) Meter {
	m := &meter{
		ticker:        ifutil.FirstOrDefault(provider, timeutil.DefaultTicker),
		data:          &meterData{},
		movingAverage: metrics.NewEWMA1(),
		start:         utc.Now(),
	}
	m.UpdateGoal(goal)
	m.ticker.Register(m)
	return m
}

// ---------------------------------------------------------------------------------------------------------------------

// meter is the default implementation of Meter.
type meter struct {
	ticker        timeutil.Ticker
	lock          sync.RWMutex
	data          *meterData
	movingAverage metrics.EWMA
	start         utc.UTC
	goal          atomic.Int64
	stopped       atomic.Bool
}

func (m *meter) Started() utc.UTC {
	return m.start
}

func (m *meter) Goal() int64 {
	return m.goal.Load()
}

func (m *meter) UpdateGoal(newGoal int64) {
	if m.stopped.Load() {
		return
	}
	m.goal.Store(newGoal)
}

func (m *meter) Stop() {
	if !m.stopped.CompareAndSwap(false, true) {
		return
	}
	m.ticker.Unregister(m)
	m.tick()
}

func (m *meter) Count() int64 {
	return m.data.totalCount.Load()
}

func (m *meter) Mark(n int64) {
	if m.stopped.Load() {
		return
	}
	m.data.totalCount.Add(n)
	m.movingAverage.Update(n)
}

func (m *meter) Rate() float64 {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.data.rate
}

func (m *meter) RateMean() float64 {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return m.data.rateMean
}

func (m *meter) Tick() {
	if m.stopped.Load() {
		return
	}
	m.tick()
}

func (m *meter) tick() {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.movingAverage.Tick()
	m.data.updateRates(m.movingAverage.Rate(), m.start)

	if m.Count() > m.goal.Load() {
		m.goal.Store(m.Count())
	}
}

func (m *meter) Snapshot() Meter {
	m.lock.RLock()
	defer m.lock.RUnlock()

	snapshot := &meter{
		data:          m.data.Snapshot(),
		movingAverage: m.movingAverage.Snapshot(),
		start:         m.start,
	}
	snapshot.goal.Store(m.goal.Load())
	if snapshot.Count() > snapshot.goal.Load() {
		snapshot.goal.Store(snapshot.Count())
	}
	snapshot.stopped.Store(true)

	return snapshot
}

func (m *meter) ETC() (utc.UTC, time.Duration) {
	now := utc.Now()

	rate := m.Rate() // in x/s
	if rate == 0 {
		return utc.Zero, 0
	}

	remaining := m.goal.Load() - m.Count()
	if remaining <= 0 {
		return now, 0
	}

	seconds := float64(remaining) / rate
	etr := time.Duration(seconds * float64(time.Second))
	return now.Add(etr), etr
}

func (m *meter) Fields() []any {
	s := m.Snapshot()
	// remaining := s.Goal() - s.Count()
	etc, etr := s.ETC()
	goal := strconv.FormatInt(s.Goal(), 10)
	return []any{
		"progress", fmt.Sprintf("%s (%*s/%s)", Percentage(s.Count(), s.Goal()), len(goal), strconv.FormatInt(s.Count(), 10), goal),
		"duration", fmt.Sprintf("%-6s", duration.Spec(utc.Since(s.Started())).RoundTo(2)),
		"etc", etc.Round(time.Minute).Format(utc.ISO8601NoSec),
		"etr", fmt.Sprintf("%6s", duration.Spec(etr).RoundTo(2)),
	}
}

func (m *meter) String() string {
	sb := strings.Builder{}
	fields := m.Fields()
	for i := 0; i+1 < len(fields); i += 2 {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(stringutil.AsString(fields[i]))
		sb.WriteString("=")
		sb.WriteString(stringutil.AsString(fields[i+1]))
	}
	return sb.String()
}

// ---------------------------------------------------------------------------------------------------------------------

// meterData is a struct that holds the data for the meter.
type meterData struct {
	totalCount     atomic.Int64 // total event count
	rate, rateMean float64      // updated under meter's write lock
}

// Snapshot returns a read-only copy of this struct.
func (m *meterData) Snapshot() *meterData {
	snapshot := &meterData{
		rate:     m.rate,
		rateMean: m.rateMean,
	}
	snapshot.totalCount.Store(m.totalCount.Load())
	return snapshot
}

func (m *meterData) updateRates(rate float64, start utc.UTC) {
	m.rate = rate
	seconds := utc.Since(start).Seconds()
	if seconds > 0 {
		m.rateMean = float64(m.totalCount.Load()) / seconds
	}
}

// ---------------------------------------------------------------------------------------------------------------------

// Percentage computes the percentage of the given value compared to total, and encodes it as a string. At least two
// digits of precision are printed.
func Percentage(value, total int64) string {
	var ratio float64
	if total != 0 {
		ratio = math.Abs(float64(value)/float64(total)) * 100
	}
	switch {
	case math.Abs(ratio) >= 99.95 && math.Abs(ratio) <= 100.05:
		return "  100%"
	case math.Abs(ratio) >= 1.0:
		return fmt.Sprintf("%5.2f%%", ratio)
	default:
		return fmt.Sprintf("%5.2g%%", ratio)
	}
}
