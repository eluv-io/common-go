package ioutil_test

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/ioutil"
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
	assertStats(t, start, stop.Sub(start), 6, r.seekSleep, r.writeSleep, r.readSleep*3+r.writeSleep*2+r.seekSleep, 3, 2, 1, s)
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
	assertStats(t, start, stop.Sub(start), 3, r.writeSleep, r.writeSleep, r.writeSleep*3, 0, 3, 0, s)
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
	reads uint64, writes uint64, seeks uint64,
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
	assert.InDelta(T, float64(statsSum)/statsCount/float64(time.Millisecond), stats["mean"].(float64), float64(durDelta)/float64(time.Millisecond))
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
