package progress_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/progress"
	"github.com/eluv-io/common-go/util/timeutil"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

func TestMeter(t *testing.T) {
	testMeter(t)
}

func testMeter(t require.TestingT) {
	var log = elog.New(&elog.Config{Handler: "text", Level: "debug"}) // needed so Example test can capture log output

	interval := time.Second * 5
	goal := int64(60)

	now := utc.UnixMilli(0)
	defer utc.MockNowFn(func() utc.UTC {
		return now
	})()

	ticker := timeutil.NewManualTicker()
	m := progress.NewMeter(goal, ticker)

	log.Info("after creation", m.Fields()...)

	now = now.Add(interval)
	ticker.Tick()

	require.Equal(t, float64(0), m.Rate())
	require.Equal(t, float64(0), m.RateMean())

	m.Mark(10)
	now = now.Add(interval)
	ticker.Tick()

	lastRate := float64(0)
	for i := int64(10); i < goal; i += 5 {
		log.Info("in progress", m.Fields()...)
		require.Greater(t, float64(1), m.Rate())
		require.Equal(t, float64(1), m.RateMean())
		require.Equal(t, i, m.Count())
		require.Less(t, lastRate, m.Rate())

		lastRate = m.Rate()
		m.Mark(5)
		now = now.Add(interval)
		ticker.Tick()
	}

	// we have reached the goal
	log.Info("at goal", m.Fields()...)
	require.Equal(t, goal, m.Count())

	// go beyond the goal: expect the goal to be updated automatically by Tick()
	m.Mark(5)
	now = now.Add(interval)
	log.Info("beyond goal, before tick", m.Fields()...)

	ticker.Tick()

	log.Info("beyond goal, after tick", m.Fields()...)
	require.Equal(t, goal+5, m.Count())
	require.Equal(t, goal+5, m.Goal())

	require.Equal(t, time.Duration(0), etr(m))

	m.UpdateGoal(goal + 10)
	log.Info("updated goal", m.Fields()...)
	require.Equal(t, goal+10, m.Goal())
	require.Less(t, 5*time.Second, etr(m))

	m.Mark(5)
	now = now.Add(interval)
	ticker.Tick()

	log.Info("reached updated goal", m.Fields()...)
	require.Equal(t, goal+10, m.Goal())
	require.Equal(t, time.Duration(0), etr(m))
}

func ExampleMeter() {
	testMeter(&MockT{})

	// Output:
	// 1970-01-01T00:00:00.000Z INFO  after creation            logger=/ progress=    0% ( 0/60) duration=0s     etc=0001-01-01T00:00Z etr=    0s
	// 1970-01-01T00:00:10.000Z INFO  in progress               logger=/ progress=16.67% (10/60) duration=10s    etc=1970-01-01T00:05Z etr= 5m13s
	// 1970-01-01T00:00:15.000Z INFO  in progress               logger=/ progress=25.00% (15/60) duration=15s    etc=1970-01-01T00:04Z etr= 3m18s
	// 1970-01-01T00:00:20.000Z INFO  in progress               logger=/ progress=33.33% (20/60) duration=20s    etc=1970-01-01T00:03Z etr= 2m18s
	// 1970-01-01T00:00:25.000Z INFO  in progress               logger=/ progress=41.67% (25/60) duration=25s    etc=1970-01-01T00:02Z etr= 1m41s
	// 1970-01-01T00:00:30.000Z INFO  in progress               logger=/ progress=50.00% (30/60) duration=30s    etc=1970-01-01T00:02Z etr= 1m15s
	// 1970-01-01T00:00:35.000Z INFO  in progress               logger=/ progress=58.33% (35/60) duration=35s    etc=1970-01-01T00:02Z etr=56.03s
	// 1970-01-01T00:00:40.000Z INFO  in progress               logger=/ progress=66.67% (40/60) duration=40s    etc=1970-01-01T00:01Z etr=40.78s
	// 1970-01-01T00:00:45.000Z INFO  in progress               logger=/ progress=75.00% (45/60) duration=45s    etc=1970-01-01T00:01Z etr=28.24s
	// 1970-01-01T00:00:50.000Z INFO  in progress               logger=/ progress=83.33% (50/60) duration=50s    etc=1970-01-01T00:01Z etr=17.58s
	// 1970-01-01T00:00:55.000Z INFO  in progress               logger=/ progress=91.67% (55/60) duration=55s    etc=1970-01-01T00:01Z etr= 8.29s
	// 1970-01-01T00:01:00.000Z INFO  at goal                   logger=/ progress=  100% (60/60) duration=1m     etc=1970-01-01T00:01Z etr=    0s
	// 1970-01-01T00:01:05.000Z INFO  beyond goal, before tick  logger=/ progress=  100% (60/60) duration=1m5s   etc=1970-01-01T00:01Z etr=    0s
	// 1970-01-01T00:01:05.000Z INFO  beyond goal, after tick   logger=/ progress=  100% (65/65) duration=1m5s   etc=1970-01-01T00:01Z etr=    0s
	// 1970-01-01T00:01:05.000Z INFO  updated goal              logger=/ progress=92.86% (65/70) duration=1m5s   etc=1970-01-01T00:01Z etr= 7.53s
	// 1970-01-01T00:01:10.000Z INFO  reached updated goal      logger=/ progress=  100% (70/70) duration=1m10s  etc=1970-01-01T00:01Z etr=    0s
}

func etr(m progress.Meter) time.Duration {
	_, etr := m.ETC()
	return etr
}

type MockT struct{}

func (t *MockT) FailNow() {}

func (t *MockT) Errorf(string, ...interface{}) {}
