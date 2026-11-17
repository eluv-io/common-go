package ioutil_test

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/ioutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
)

const durDelta = time.Millisecond * 10

func TestStatsLogger(t *testing.T) {
	var stats, slows []interface{}
	statsLog := func(msg string, fields ...interface{}) {
		stats = append([]interface{}{msg}, fields...)
	}
	slowsLog := func(msg string, fields ...interface{}) {
		slows = append([]interface{}{msg}, fields...)
	}

	fs := afero.NewMemMapFs()
	f, err := fs.Create("test.txt")
	require.NoError(t, err)

	r := &sleeper{r: f,
		readSleep:  time.Millisecond * 200,
		writeSleep: time.Millisecond * 300,
		seekSleep:  time.Millisecond * 100,
	}
	start := utc.Now()
	limitFn := func(fn int, off int64) time.Duration {
		if fn == ioutil.ReadFn {
			return time.Millisecond * 150
		}
		return time.Millisecond * 250
	}
	op := "TestStatsLogger"
	fields := []interface{}{"elu", "vio"}

	l := ioutil.NewStatsLogger(r, statsLog, slowsLog, start, limitFn, op, fields...)

	d := []byte("helloworld")
	x := len(d) / 2
	n, err := l.Write(d)
	require.NoError(t, err)
	require.Equal(t, len(d), n)
	l.Record("first_write", "1")
	n2, err := l.Seek(0, io.SeekStart)
	require.NoError(t, err)
	require.Equal(t, int64(0), n2)
	p := make([]byte, x)
	n, err = l.Read(p)
	require.NoError(t, err)
	require.Equal(t, len(p), n)
	require.Equal(t, d[:n], p)
	l.Record("first_read", "1")
	n, err = l.Read(p)
	require.NoError(t, err)
	require.Equal(t, len(p), n)
	require.Equal(t, d[n:], p)
	l.Record("second_read", "2")
	n, err = l.Write([]byte{})
	require.NoError(t, err)
	require.Zero(t, n)
	l.Record("last_write", "2")
	n, err = l.Read(p)
	require.Equal(t, io.EOF, err)
	require.Zero(t, n)
	l.Record("last_read", "3")
	err = l.Close()
	require.NoError(t, err)
	stop := utc.Now()

	assert.Len(t, stats, 7)
	assert.Equal(t, op+" stats", stats[0])
	assert.Equal(t, fields[0], stats[1])
	assert.Equal(t, fields[1], stats[2])
	assert.Equal(t, "stats", stats[3])
	s := toMap(t, stats[4])
	assertStats(t, start, stop.Sub(start), 6, r.seekSleep, r.writeSleep, r.readSleep*3+r.writeSleep*2+r.seekSleep, 3, 2, 1, int64(len(d)), int64(len(d)), 0, s)
	assert.Equal(t, "events", stats[5])
	e := toMapSlice(t, stats[6])
	assert.Len(t, e, 5)
	dur := r.writeSleep
	assertEvent(t, start.Add(dur), "first_write", 0, dur, 0, int64(len(d)), "1", e[0])
	dur += r.seekSleep + r.readSleep
	assertEvent(t, start.Add(dur), "first_read", 0, dur, 0, int64(x), "1", e[1])
	dur += r.readSleep
	assertEvent(t, start.Add(dur), "second_read", 0, dur, int64(x), int64(x), "2", e[2])
	dur += r.writeSleep
	assertEvent(t, start.Add(dur), "last_write", 0, dur, int64(len(d)), 0, "2", e[3])
	dur += r.readSleep
	assertEvent(t, start.Add(dur), "last_read", 0, dur, int64(len(d)), 0, "3", e[4])

	assert.Len(t, slows, 7)
	assert.Equal(t, op+" slow operations", slows[0])
	assert.Equal(t, fields[0], slows[1])
	assert.Equal(t, fields[1], slows[2])
	assert.Equal(t, "limit", slows[3])
	assert.Equal(t, duration.Spec(limitFn(ioutil.ReadFn, 0)), slows[4])
	assert.Equal(t, "ops", slows[5])
	o := toMapSlice(t, slows[6])
	assert.Len(t, o, 5)
	dur = r.writeSleep
	assertEvent(t, start.Add(dur), "write", limitFn(ioutil.WriteFn, 0), r.writeSleep, 0, int64(len(d)), nil, o[0])
	dur += r.seekSleep + r.readSleep
	assertEvent(t, start.Add(dur), "read", 0, r.readSleep, 0, int64(x), nil, o[1])
	dur += r.readSleep
	assertEvent(t, start.Add(dur), "read", 0, r.readSleep, int64(x), int64(x), nil, o[2])
	dur += r.writeSleep
	assertEvent(t, start.Add(dur), "write", limitFn(ioutil.WriteFn, 0), r.writeSleep, int64(len(d)), 0, nil, o[3])
	dur += r.readSleep
	assertEvent(t, start.Add(dur), "read", 0, r.readSleep, int64(len(d)), 0, nil, o[4])
}

func TestStatsLogger_Same(t *testing.T) {
	var stats, slows []interface{}
	statsLog := func(msg string, fields ...interface{}) {
		stats = append([]interface{}{msg}, fields...)
	}
	slowsLog := func(msg string, fields ...interface{}) {
		slows = append([]interface{}{msg}, fields...)
	}

	fs := afero.NewMemMapFs()
	f, err := fs.Create("test.txt")
	require.NoError(t, err)

	r := &sleeper{r: f,
		writeSleep: time.Millisecond * 100,
	}
	start := utc.Now()
	limitFn := func(fn int, off int64) time.Duration {
		return time.Millisecond * 100
	}
	op := "TestStatsLogger"
	fields := []interface{}{"elu", "vio"}

	l := ioutil.NewStatsLogger(r, statsLog, slowsLog, start, limitFn, op, fields...)

	d := []byte("helloworld")
	x := len(d) / 2
	n, err := l.Write(d[:x])
	require.NoError(t, err)
	require.Equal(t, x, n)
	l.Record("first_write", "first")
	n, err = l.Write(d[x:])
	require.NoError(t, err)
	require.Equal(t, x, n)
	n, err = l.Write([]byte{})
	require.NoError(t, err)
	require.Zero(t, n)
	l.Record("last_write", "last")
	err = l.Close()
	require.NoError(t, err)
	stop := utc.Now()

	assert.Len(t, stats, 7)
	assert.Equal(t, op+".Write stats", stats[0])
	assert.Equal(t, fields[0], stats[1])
	assert.Equal(t, fields[1], stats[2])
	assert.Equal(t, "stats", stats[3])
	s := toMap(t, stats[4])
	assertStats(t, start, stop.Sub(start), 3, r.writeSleep, r.writeSleep, r.writeSleep*3, 0, 3, 0, 0, int64(len(d)), 0, s)
	assert.Equal(t, "events", stats[5])
	e := toMapSlice(t, stats[6])
	assert.Len(t, e, 2)
	dur := r.writeSleep
	assertEvent(t, start.Add(dur), "first_write", 0, dur, 0, int64(x), "first", e[0])
	dur += r.writeSleep * 2
	assertEvent(t, start.Add(dur), "last_write", 0, dur, int64(len(d)), 0, "last", e[1])

	assert.Len(t, slows, 7)
	assert.Equal(t, op+".Write slow operations", slows[0])
	assert.Equal(t, fields[0], slows[1])
	assert.Equal(t, fields[1], slows[2])
	assert.Equal(t, "limit", slows[3])
	assert.Equal(t, duration.Spec(limitFn(ioutil.WriteFn, 0)), slows[4])
	assert.Equal(t, "ops", slows[5])
	o := toMapSlice(t, slows[6])
	assert.Len(t, o, 3)
	dur = r.writeSleep
	assertEvent(t, start.Add(dur), "", 0, r.writeSleep, 0, int64(x), nil, o[0])
	dur += r.writeSleep
	assertEvent(t, start.Add(dur), "", 0, r.writeSleep, int64(x), int64(x), nil, o[1])
	dur += r.writeSleep
	assertEvent(t, start.Add(dur), "", 0, r.writeSleep, int64(len(d)), 0, nil, o[2])
}

// TestStatsLogger_ConcurrentRecord verifies that Record may be called concurrently with Read/Write/Seek and Close.
// Its main value is when run with -race.
func TestStatsLogger_ConcurrentRecord(t *testing.T) {
	var stats, slows []interface{}
	statsLog := func(msg string, fields ...interface{}) {
		stats = append([]interface{}{msg}, fields...)
	}
	slowsLog := func(msg string, fields ...interface{}) {
		slows = append([]interface{}{msg}, fields...)
	}

	fs := afero.NewMemMapFs()
	f, err := fs.Create("test.txt")
	require.NoError(t, err)

	start := utc.Now()
	// every duration exceeds a negative limit, so every operation is "slow" (even if the clock measures 0), which also
	// exercises the slow-event path
	limitFn := func(fn int, off int64) time.Duration { return -1 }
	l := ioutil.NewStatsLogger(&sleeper{r: f}, statsLog, slowsLog, start, limitFn, "TestStatsLogger")

	const recorders = 4
	var (
		wg       sync.WaitGroup
		stop     atomic.Bool
		recorded atomic.Int64
	)
	for i := 0; i < recorders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; !stop.Load(); j++ {
				if j%2 == 0 {
					l.Record("event", j)
				} else {
					l.Record("event", j, utc.Now())
				}
				recorded.Add(1)
				runtime.Gosched() // don't starve the main goroutine (or grow events unboundedly) with GOMAXPROCS=1
			}
		}()
	}

	d := []byte("helloworld")
	p := make([]byte, len(d))
	for i := 0; i < 200; i++ {
		_, err = l.Seek(0, io.SeekStart)
		require.NoError(t, err)
		_, err = l.Write(d)
		require.NoError(t, err)
		_, err = l.Seek(0, io.SeekStart)
		require.NoError(t, err)
		_, err = l.Read(p)
		require.NoError(t, err)
	}

	// make sure the recorders have actually started (they might not have been scheduled yet, e.g. with GOMAXPROCS=1)
	for recorded.Load() == 0 {
		runtime.Gosched()
	}

	// Close while the recorders are still running
	require.NoError(t, l.Close())
	stop.Store(true)
	wg.Wait()

	// events recorded after Close are dropped: no panic, and the logged events remain untouched
	require.Len(t, stats, 5) // msg, "stats", stats, "events", events
	events := toMapSlice(t, stats[4])
	l.Record("late", nil)
	require.Len(t, toMapSlice(t, stats[4]), len(events))
	assert.LessOrEqual(t, int64(len(events)), recorded.Load())
	for _, e := range events {
		assert.Equal(t, "event", e["name"])
		// (off, n) must be the result of a single Read/Write/Seek, never a mix of two. The loop only produces:
		// (0, 0) before the first op, (10, 0) for a seek to start after a write/read, (0, 10) for a write/read at 0.
		off, n := int64(e["off"].(float64)), int64(e["n"].(float64))
		assert.Contains(t, [][2]int64{{0, 0}, {int64(len(d)), 0}, {0, int64(len(d))}}, [2]int64{off, n})
	}
	require.NotEmpty(t, slows)
}

// statsLogs captures what a StatsLogger logs on Close.
type statsLogs struct {
	stats      []interface{} // msg, [fields...], "stats", stats, "events", events
	slows      []interface{} // msg, [fields...], ["limit", limit], "ops", ops
	statsCalls int
	slowsCalls int
}

func (g *statsLogs) statsLog(msg string, fields ...interface{}) {
	g.statsCalls++
	g.stats = append([]interface{}{msg}, fields...)
}

func (g *statsLogs) slowsLog(msg string, fields ...interface{}) {
	g.slowsCalls++
	g.slows = append([]interface{}{msg}, fields...)
}

// seekFails is a reader whose Seek always fails and returns 0 - which is what many implementations do on error.
type seekFails struct{}

// Both methods sleep a little: tests that use a limit of 0 to make every call "slow" need a measurable, non-zero
// duration, which a call that returns immediately doesn't have on a coarse clock.
func (seekFails) Read(p []byte) (int, error) {
	time.Sleep(time.Millisecond)
	return len(p), nil
}

func (seekFails) Seek(int64, int) (int64, error) {
	time.Sleep(time.Millisecond)
	return 0, errors.Str("seek failed")
}

// TestStatsLogger_FailedSeek verifies that a failed Seek does not reset the tracked offset.
func TestStatsLogger_FailedSeek(t *testing.T) {
	g := &statsLogs{}
	limitFn := func(int, int64) time.Duration { return time.Hour }
	l := ioutil.NewStatsLogger(seekFails{}, g.statsLog, g.slowsLog, utc.Now(), limitFn, "op")

	_, err := l.Read(make([]byte, 100))
	require.NoError(t, err)
	_, err = l.Seek(5, io.SeekStart)
	require.Error(t, err)
	l.Record("after-failed-seek", nil)
	_, err = l.Read(make([]byte, 10))
	require.NoError(t, err)
	l.Record("after-read", nil)
	require.NoError(t, l.Close())

	// the failed seek is still counted, but the position stays at 100
	s := toMap(t, g.stats[2])
	assert.EqualValues(t, 2, s["reads"])
	assert.EqualValues(t, 1, s["seeks"])
	assert.EqualValues(t, 110, s["rbytes"])
	assert.EqualValues(t, 100, s["sbytes"])

	e := toMapSlice(t, g.stats[4])
	require.Len(t, e, 2)
	assert.EqualValues(t, 100, e[0]["off"])
	assert.EqualValues(t, 100, e[0]["n"])
	assert.EqualValues(t, 100, e[1]["off"], "offset of the read after a failed seek")
	assert.EqualValues(t, 10, e[1]["n"])
}

// TestStatsLogger_FailedSeekSlow verifies that a slow, failed Seek is still reported with the unchanged position.
func TestStatsLogger_FailedSeekSlow(t *testing.T) {
	g := &statsLogs{}
	l := ioutil.NewStatsLogger(seekFails{}, g.statsLog, g.slowsLog, utc.Now(),
		func(int, int64) time.Duration { return 0 }, "op")
	_, err := l.Read(make([]byte, 100))
	require.NoError(t, err)
	_, err = l.Seek(5, io.SeekStart)
	require.Error(t, err)
	require.NoError(t, l.Close())

	require.NotEmpty(t, g.slows)
	ops := toMapSlice(t, g.slows[len(g.slows)-1])
	require.Len(t, ops, 2)
	assert.EqualValues(t, 100, ops[1]["off"])
	assert.EqualValues(t, 100, ops[1]["n"])
}

// TestStatsLogger_Unsupported verifies that calling Read/Write/Seek on a value that doesn't implement the respective
// interface returns an error instead of panicking, and that such calls are not recorded.
func TestStatsLogger_Unsupported(t *testing.T) {
	limitFn := func(int, int64) time.Duration { return 0 } // would report any recorded call as slow
	tests := []struct {
		name    string
		wrapped interface{}
		call    func(l *ioutil.StatsLogger) error
	}{
		{"read on writer", struct{ io.Writer }{io.Discard}, func(l *ioutil.StatsLogger) error {
			_, err := l.Read(make([]byte, 1))
			return err
		}},
		{"write on reader", struct{ io.Reader }{strings.NewReader("x")}, func(l *ioutil.StatsLogger) error {
			_, err := l.Write([]byte("x"))
			return err
		}},
		{"seek on reader", struct{ io.Reader }{strings.NewReader("x")}, func(l *ioutil.StatsLogger) error {
			_, err := l.Seek(0, io.SeekStart)
			return err
		}},
		{"read on nil", nil, func(l *ioutil.StatsLogger) error {
			_, err := l.Read(make([]byte, 1))
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &statsLogs{}
			l := ioutil.NewStatsLogger(tt.wrapped, g.statsLog, g.slowsLog, utc.Now(), limitFn, "myop")

			var err error
			require.NotPanics(t, func() { err = tt.call(l) })
			require.Error(t, err)
			assert.True(t, errors.IsKind(errors.K.Invalid, err), "kind: %v", err)
			assert.Contains(t, err.Error(), "StatsLogger.", "must name the failed method: %v", err)
			assert.Contains(t, err.Error(), "myop", "must name the logger: %v", err)

			require.NoError(t, l.Close())
			assert.EqualValues(t, 0, toMap(t, g.stats[2])["count"])
			assert.Nil(t, g.slows, "unsupported calls must not be reported as slow")
		})
	}
}

// TestStatsLogger_NilLimitFn verifies that a nil limitFn means "no limit" instead of panicking.
func TestStatsLogger_NilLimitFn(t *testing.T) {
	g := &statsLogs{}
	f, err := afero.NewMemMapFs().Create("test.txt")
	require.NoError(t, err)
	const d = 20 * time.Millisecond
	r := &sleeper{r: f, readSleep: d, writeSleep: d, seekSleep: d}
	l := ioutil.NewStatsLogger(r, g.statsLog, g.slowsLog, utc.Now(), nil, "op")

	require.NotPanics(t, func() {
		_, err = l.Write([]byte("hello"))
		require.NoError(t, err)
		_, err = l.Seek(0, io.SeekStart)
		require.NoError(t, err)
		_, err = l.Read(make([]byte, 5))
		require.NoError(t, err)
	})
	require.NoError(t, l.Close())

	assert.EqualValues(t, 3, toMap(t, g.stats[2])["count"])
	assert.Nil(t, g.slows, "no operation may be reported as slow without a limit")
}

// TestStatsLogger_AfterClose verifies that calls after Close neither modify what was handed to the loggers nor record
// events.
func TestStatsLogger_AfterClose(t *testing.T) {
	g := &statsLogs{}
	f, err := afero.NewMemMapFs().Create("test.txt")
	require.NoError(t, err)
	// hide f's Close method, so that Close() doesn't close the file and calls after Close() succeed
	rws := struct{ io.ReadWriteSeeker }{f}
	l := ioutil.NewStatsLogger(rws, g.statsLog, g.slowsLog, utc.Now(),
		func(int, int64) time.Duration { return 0 }, "op")

	_, err = l.Write([]byte("hello"))
	require.NoError(t, err)
	l.Record("before", nil)
	require.NoError(t, l.Close())

	statsBefore := fmt.Sprint(g.stats[2])
	events := toMapSlice(t, g.stats[4])
	require.Len(t, events, 1)
	ops := toMapSlice(t, g.slows[len(g.slows)-1])
	require.Len(t, ops, 1)
	require.Equal(t, 1, g.statsCalls)
	require.Equal(t, 1, g.slowsCalls)

	// none of these may panic or modify the already logged data
	require.NotPanics(t, func() {
		_, err = l.Write([]byte("world"))
		require.NoError(t, err)
		_, err = l.Seek(0, io.SeekStart)
		require.NoError(t, err)
		l.Record("after", nil)
		l.Record("after", nil, utc.Now())
		require.NoError(t, l.Close())
	})

	assert.Equal(t, statsBefore, fmt.Sprint(g.stats[2]), "logged stats must be a snapshot")
	assert.Len(t, toMapSlice(t, g.stats[4]), 1)
	assert.Len(t, toMapSlice(t, g.slows[len(g.slows)-1]), 1)
	assert.Equal(t, 1, g.statsCalls, "second Close must not log again")
	assert.Equal(t, 1, g.slowsCalls, "second Close must not log again")
	assert.Equal(t, "before", toMapSlice(t, g.stats[4])[0]["name"])
}

// TestStatsLogger_FieldsNotModified verifies that Close does not write into the backing array of the caller's fields.
func TestStatsLogger_FieldsNotModified(t *testing.T) {
	g := &statsLogs{}
	f, err := afero.NewMemMapFs().Create("test.txt")
	require.NoError(t, err)
	fields := make([]interface{}, 2, 16) // spare capacity: append would write into it
	fields[0], fields[1] = "elu", "vio"
	l := ioutil.NewStatsLogger(&sleeper{r: f, writeSleep: time.Millisecond}, g.statsLog, g.slowsLog, utc.Now(),
		func(int, int64) time.Duration { return 0 }, "op", fields...)

	_, err = l.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, l.Close())

	for i, v := range fields[:cap(fields)][2:] {
		assert.Nil(t, v, "fields backing array modified at index %d", i+2)
	}
	// and the slow operations log (which also starts from the caller's fields) is intact
	assert.Equal(t, "elu", g.slows[1])
	assert.Equal(t, "vio", g.slows[2])
	assert.Equal(t, "elu", g.stats[1])
	assert.Equal(t, "vio", g.stats[2])
}

// TestStatsLogger_NilLoggers verifies that nil log functions discard the logs instead of panicking in Close.
func TestStatsLogger_NilLoggers(t *testing.T) {
	tests := []struct {
		name     string
		statsLog bool
		slowsLog bool
	}{
		{"both nil", false, false},
		{"stats nil", false, true},
		{"slows nil", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &statsLogs{}
			var statsLog, slowsLog func(string, ...interface{})
			if tt.statsLog {
				statsLog = g.statsLog
			}
			if tt.slowsLog {
				slowsLog = g.slowsLog
			}
			f, err := afero.NewMemMapFs().Create("test.txt")
			require.NoError(t, err)
			// a limit of 0 makes every operation slow, so both logs have something to write
			l := ioutil.NewStatsLogger(&sleeper{r: f, writeSleep: time.Millisecond}, statsLog, slowsLog, utc.Now(),
				func(int, int64) time.Duration { return 0 }, "op")
			_, err = l.Write([]byte("hello"))
			require.NoError(t, err)
			l.Record("event", nil)

			require.NotPanics(t, func() { require.NoError(t, l.Close()) })
			if tt.statsLog {
				assert.Equal(t, 1, g.statsCalls)
			}
			if tt.slowsLog {
				assert.Equal(t, 1, g.slowsCalls)
			}
		})
	}
}

type sleeper struct {
	r          ioutil.ReadWriteSeekCloser
	readSleep  time.Duration
	writeSleep time.Duration
	seekSleep  time.Duration
}

func (s *sleeper) Read(p []byte) (int, error) {
	time.Sleep(s.readSleep)
	return s.r.Read(p)
}

func (s *sleeper) Write(p []byte) (int, error) {
	time.Sleep(s.writeSleep)
	return s.r.Write(p)
}

func (s *sleeper) Seek(offset int64, whence int) (int64, error) {
	time.Sleep(s.seekSleep)
	return s.r.Seek(offset, whence)
}

func (s *sleeper) Close() error {
	return s.r.Close()
}

func assertStats(T *testing.T,
	start utc.UTC, dur time.Duration, count uint64, min time.Duration, max time.Duration, sum time.Duration,
	reads int, writes int, seeks int, rbytes int64, wbytes int64, sbytes int64,
	stats map[string]interface{},
) {
	require.NotNil(T, stats)
	eventStart, err := utc.FromString(stats["start"].(string))
	require.NoError(T, err)
	assert.WithinDuration(T, start.Time, eventStart.Time, durDelta)
	statsDur, err := duration.FromString(stats["duration"].(string))
	require.NoError(T, err)
	assert.InDelta(T, int64(dur), int64(statsDur), float64(durDelta))
	statsCount := stats["count"].(float64)
	assert.Equal(T, float64(count), statsCount)
	statsMin, err := duration.FromString(stats["min"].(string))
	require.NoError(T, err)
	assert.InDelta(T, int64(min), int64(statsMin), float64(durDelta))
	statsMax, err := duration.FromString(stats["max"].(string))
	require.NoError(T, err)
	assert.InDelta(T, int64(max), int64(statsMax), float64(durDelta))
	statsSum, err := duration.FromString(stats["sum"].(string))
	require.NoError(T, err)
	assert.InDelta(T, int64(sum), int64(statsSum), float64(durDelta))
	statsMean, err := duration.FromString(stats["mean"].(string))
	require.NoError(T, err)
	assert.InDelta(T, float64(statsSum)/statsCount, float64(statsMean), float64(durDelta))
	if reads > 0 {
		assert.Equal(T, float64(reads), stats["reads"].(float64))
	} else {
		assert.Nil(T, stats["reads"])
	}
	if writes > 0 {
		assert.Equal(T, float64(writes), stats["writes"].(float64))
	} else {
		assert.Nil(T, stats["writes"])
	}
	if seeks > 0 {
		assert.Equal(T, float64(seeks), stats["seeks"].(float64))
	} else {
		assert.Nil(T, stats["seeks"])
	}
	if rbytes > 0 {
		assert.Equal(T, float64(rbytes), stats["rbytes"].(float64))
	} else {
		assert.Nil(T, stats["rbytes"])
	}
	if wbytes > 0 {
		assert.Equal(T, float64(wbytes), stats["wbytes"].(float64))
	} else {
		assert.Nil(T, stats["wbytes"])
	}
	if sbytes > 0 {
		assert.Equal(T, float64(sbytes), stats["sbytes"].(float64))
	} else {
		assert.Nil(T, stats["sbytes"])
	}
}

func assertEvent(T *testing.T,
	t utc.UTC, name string, limit time.Duration, dur time.Duration, off int64, n int64, data interface{},
	event map[string]interface{},
) {
	require.NotNil(T, event)
	eventT, err := utc.FromString(event["t"].(string))
	require.NoError(T, err)
	assert.WithinDuration(T, t.Time, eventT.Time, durDelta)
	if name != "" {
		assert.Equal(T, name, event["name"])
	} else {
		assert.Nil(T, event["name"])
	}
	if limit > 0 {
		eventLimit, err := duration.FromString(event["limit"].(string))
		require.NoError(T, err)
		assert.InDelta(T, int64(limit), int64(eventLimit), float64(durDelta))
	} else {
		assert.Nil(T, event["limit"])
	}
	eventDur, err := duration.FromString(event["dur"].(string))
	require.NoError(T, err)
	assert.InDelta(T, int64(dur), int64(eventDur), float64(durDelta))
	assert.Equal(T, float64(off), event["off"].(float64))
	assert.Equal(T, float64(n), event["n"].(float64))
	assert.Equal(T, data, event["data"])
}

func toMap(t *testing.T, d interface{}) map[string]interface{} {
	var res map[string]interface{}
	require.NotNil(t, d)
	b, err := json.Marshal(d)
	require.NoError(t, err)
	require.NotEmpty(t, b)
	err = json.Unmarshal(b, &res)
	require.NoError(t, err)
	return res
}

func toMapSlice(t *testing.T, d interface{}) []map[string]interface{} {
	var res []map[string]interface{}
	require.NotNil(t, d)
	b, err := json.Marshal(d)
	require.NoError(t, err)
	require.NotEmpty(t, b)
	err = json.Unmarshal(b, &res)
	require.NoError(t, err)
	return res
}
