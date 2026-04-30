package statsutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/utc-go"
)

func TestStatistics_Update(t *testing.T) {
	stats := Statistics[int]{}
	now := utc.Now()

	// Test first update
	stats.Update(now, 5)
	assert.Equal(t, now, stats.Start)
	assert.Equal(t, int64(0), int64(stats.Duration))
	assert.Equal(t, uint64(1), stats.Count)
	assert.Equal(t, 5, stats.Min)
	assert.Equal(t, 5, stats.Max)
	assert.Equal(t, 5, stats.Sum)
	assert.Equal(t, float64(5), stats.Mean)

	// Test subsequent updates
	stats.Update(now.Add(time.Second), 3)
	assert.Equal(t, now, stats.Start)
	assert.Equal(t, duration.Second, stats.Duration)
	assert.Equal(t, uint64(2), stats.Count)
	assert.Equal(t, 3, stats.Min)
	assert.Equal(t, 5, stats.Max)
	assert.Equal(t, 8, stats.Sum)
	assert.Equal(t, float64(4), stats.Mean)
}

func TestPeriodic_UpdateNow(t *testing.T) {
	p := Periodic[int]{Period: duration.Second}
	now := utc.Now()

	// Test first update
	result := p.UpdateNow(now, 5)
	assert.False(t, result)
	assert.Equal(t, 5, p.Current.Sum)
	assert.Equal(t, 5, p.Total.Sum)

	// Test update within same period
	result = p.UpdateNow(now.Add(500*time.Millisecond), 3)
	assert.False(t, result)
	assert.Equal(t, 8, p.Current.Sum)
	assert.Equal(t, 8, p.Total.Sum)

	// Test update in new period
	result = p.UpdateNow(now.Add(1100*time.Millisecond), 4)
	assert.True(t, result)
	assert.Equal(t, duration.Second, p.Previous.Duration)
	assert.Equal(t, 8, p.Previous.Sum)
	assert.Equal(t, 4, p.Current.Sum)
	assert.Equal(t, 12, p.Total.Sum)
}

func TestPeriodic_UpdateNow_actualDuration(t *testing.T) {
	p := Periodic[int]{Period: duration.Second, ActualDuration: true}
	now := utc.Now()

	// Test first update
	result := p.UpdateNow(now, 1)
	assert.False(t, result)
	assert.Equal(t, 1, p.Current.Sum)
	assert.Equal(t, 1, p.Total.Sum)

	// Test update within same period
	result = p.UpdateNow(now.Add(500*time.Millisecond), 7)
	assert.False(t, result)
	assert.Equal(t, 8, p.Current.Sum)
	assert.Equal(t, 8, p.Total.Sum)

	// Test update in new period
	result = p.UpdateNow(now.Add(1100*time.Millisecond), 4)
	assert.True(t, result)
	assert.Equal(t, 500*duration.Millisecond, p.Previous.Duration)
	assert.Equal(t, 8, p.Previous.Sum)
	assert.Equal(t, 4, p.Current.Sum)
	assert.Equal(t, 12, p.Total.Sum)
}

func TestPeriodic_ManualSwitch(t *testing.T) {
	p := Periodic[int]{Period: duration.Second, ManualSwitch: true}
	start := utc.Now()

	// Test first update
	result := p.UpdateNow(start, 5)
	assert.False(t, result)
	assertStatistics(t, p.Current, start, 0, 5, 5, 5)
	assertStatistics(t, p.Total, start, 0, 5, 5, 5)

	// Test update within same period
	result = p.UpdateNow(start.Add(500*time.Millisecond), 3)
	assert.False(t, result)
	assertStatistics(t, p.Current, start, 500*time.Millisecond, 3, 5, 8)
	assertStatistics(t, p.Total, start, 500*time.Millisecond, 3, 5, 8)

	start2 := start.Add(time.Second)
	p.Switch(start2)

	// Test update in new period
	result = p.UpdateNow(start.Add(1100*time.Millisecond), 4)
	assert.False(t, result)
	assertStatistics(t, p.Previous, start, time.Second, 3, 5, 8)
	assertStatistics(t, p.Current, start2, 100*time.Millisecond, 4, 4, 4)
	assertStatistics(t, p.Total, start, 1100*time.Millisecond, 3, 5, 12)
}

func assertStatistics[T Number](t *testing.T, s Statistics[T], start utc.UTC, dur time.Duration, min, max, sum T) {
	assert.Equal(t, start, s.Start, "start")
	assert.Equal(t, dur, s.Duration.Duration(), "duration")
	assert.Equal(t, min, s.Min, "min")
	assert.Equal(t, max, s.Max, "max")
	assert.Equal(t, sum, s.Sum, "sum")
}
