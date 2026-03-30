package statsutil

import (
	"golang.org/x/exp/constraints"

	"github.com/eluv-io/common-go/collections/slidingwindow"
	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/utc-go"
)

type Number interface {
	constraints.Integer | constraints.Float
}

// Periodic is a utility for collecting periodic statistics. It keeps a statistics struct for the Current period, the
// Previous period, and the Total. Per default, the period is 1 second, and at the end of the period, the Current period
// statistics are copied to Previous and a new Current period is started. If ManualSwitch is true, the period will not
// switch automatically on Update, but must be switched manually by calling Switch().
type Periodic[T Number] struct {
	Period   duration.Spec `json:"period"`   // period - defaults to 1s
	Total    Statistics[T] `json:"total"`    // the running total of all values
	Current  Statistics[T] `json:"current"`  // the running (incomplete) statistics for the current period
	Previous Statistics[T] `json:"previous"` // the complete statistics for the previous period

	// options

	// if true, the period will not switch automatically on Update
	ManualSwitch bool `json:"-"`
	// if false, the Duration is set to Period, otherwise to the duration between the first and last update in the period.
	ActualDuration bool `json:"-"`
}

// Update updates the statistics with a new value and returns true if the last period has elapsed.
func (p *Periodic[T]) Update(val T) bool {
	return p.UpdateNow(utc.Now(), val)
}

// UpdateNow updates the statistics with a new value and returns true if the last period has elapsed. Now is the time
// when the value was recorded. If ManualSwitch is false, the period will switch automatically if the last period has
// elapsed. Otherwise, call Switch() manually to switch to a new period.
func (p *Periodic[T]) UpdateNow(now utc.UTC, val T) bool {
	if p.Period == 0 {
		p.Period = duration.Second
	}
	p.Total.Update(now, val)

	if !p.Current.Start.IsZero() && now.Sub(p.Current.Start) > p.Period.Duration() {
		if !p.ManualSwitch {
			p.Switch(now)
		}
		p.Current.Update(now, val)
		return true
	}

	p.Current.Update(now, val)
	return false
}

// Switch switches to a new period by finalizing the previous period and starting a new one.
func (p *Periodic[T]) Switch(now utc.UTC) {
	// finalize previous period
	p.Previous = p.Current
	if !p.ActualDuration {
		p.Previous.Duration = p.Period
	}
	// start new period
	p.Current = Statistics[T]{Start: now}
}

// Statistics is a utility for collecting statistics of a given measurement. It calculates and updates its members
// Count, Min, Max, Sum, Mean on every call to Update. In addition, it can calculate the Variance with a separate call
// to CalcVariance.
type Statistics[T Number] struct {
	Start    utc.UTC       `json:"start"`
	Duration duration.Spec `json:"duration"`
	Count    uint64        `json:"count"`
	Min      T             `json:"min"`
	Max      T             `json:"max"`
	Sum      T             `json:"sum"`
	Mean     float64       `json:"mean"`
	Variance float64       `json:"variance,omitempty"`
	m2       float64       // used for variance calculation
}

// Update updates the statistics with a new value.
func (p *Statistics[T]) Update(now utc.UTC, val T) {
	if p.Start.IsZero() {
		p.Start = now
		p.Min = val
		p.Max = val
	}
	p.Duration = duration.Spec(now.Sub(p.Start))
	p.Count++
	p.Sum += val
	if val < p.Min {
		p.Min = val
	} else if val > p.Max {
		p.Max = val
	}

	// Update mean and m2 using Welford's method
	vf64 := float64(val)
	if p.Count == 1 {
		p.Mean = vf64
		p.m2 = 0.0
	} else {
		delta := vf64 - p.Mean
		p.Mean += delta / float64(p.Count)
		p.m2 += delta * (vf64 - p.Mean)
	}
}

func (p *Statistics[T]) CalcVariance(useSampleVariance bool) {
	p.Variance = slidingwindow.Variance(useSampleVariance, p.m2, int(p.Count))
}
