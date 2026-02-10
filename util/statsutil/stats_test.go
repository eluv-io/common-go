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
