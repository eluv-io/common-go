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
	"github.com/eluv-io/common-go/util/statsutil"
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
	limit := time.Millisecond * 150
	op := "TestStatsLogger"
	fields := []interface{}{"elu", "vio"}

	l := ioutil.NewStatsLogger(r, statsLog, slowsLog, start, limit, op, fields...)

	d := []byte("helloworld")
	n, err := l.Write(d)
	require.NoError(t, err)
	require.Equal(t, len(d), n)
	l.Record("first_write", "1")
	x, err := l.Seek(0, io.SeekStart)
	require.NoError(t, err)
	require.Equal(t, int64(0), x)
	p := make([]byte, len(d)/2)
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
	s, ok := stats[4].(statsutil.Statistics[time.Duration])
	assert.True(t, ok)
	assert.NotNil(t, s)
	assert.Equal(t, start, s.Start)
	assert.InDelta(t, int64(stop.Sub(start)), int64(s.Duration), float64(durDelta))
	assert.Equal(t, uint64(6), s.Count)
	assert.InDelta(t, int64(r.seekSleep), int64(s.Min), float64(durDelta))
	assert.InDelta(t, int64(r.writeSleep), int64(s.Max), float64(durDelta))
	assert.InDelta(t, int64(r.readSleep*3+r.writeSleep*2+r.seekSleep), s.Sum, float64(durDelta))
	assert.InDelta(t, float64(s.Sum)/float64(s.Count)/float64(time.Millisecond), s.Mean, float64(durDelta)/float64(time.Millisecond))
	assert.Equal(t, "events", stats[5])
	e := toMap(t, stats[6])
	assert.Len(t, e, 5)
	dur := r.writeSleep
	assertEvent(t, "first_write", start.Add(dur), dur, 0, int64(len(d)), "1", e[0])
	dur += r.seekSleep + r.readSleep
	assertEvent(t, "first_read", start.Add(dur), dur, 0, int64(len(d)/2), "1", e[1])
	dur += r.readSleep
	assertEvent(t, "second_read", start.Add(dur), dur, int64(len(d)/2), int64(len(d)/2), "2", e[2])
	dur += r.writeSleep
	assertEvent(t, "last_write", start.Add(dur), dur, int64(len(d)), 0, "2", e[3])
	dur += r.readSleep
	assertEvent(t, "last_read", start.Add(dur), dur, int64(len(d)), 0, "3", e[4])

	assert.Len(t, slows, 7)
	assert.Equal(t, op+" slow operations", slows[0])
	assert.Equal(t, fields[0], slows[1])
	assert.Equal(t, fields[1], slows[2])
	assert.Equal(t, "limit", slows[3])
	assert.Equal(t, limit, slows[4])
	assert.Equal(t, "ops", slows[5])
	o := toMap(t, slows[6])
	assert.Len(t, o, 5)
	dur = r.writeSleep
	assertEvent(t, "Write", start.Add(dur), r.writeSleep, 0, int64(len(d)), nil, o[0])
	dur += r.seekSleep + r.readSleep
	assertEvent(t, "Read", start.Add(dur), r.readSleep, 0, int64(len(d)/2), nil, o[1])
	dur += r.readSleep
	assertEvent(t, "Read", start.Add(dur), r.readSleep, int64(len(d)/2), int64(len(d)/2), nil, o[2])
	dur += r.writeSleep
	assertEvent(t, "Write", start.Add(dur), r.writeSleep, int64(len(d)), 0, nil, o[3])
	dur += r.readSleep
	assertEvent(t, "Read", start.Add(dur), r.readSleep, int64(len(d)), 0, nil, o[4])
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

func toMap(t *testing.T, d interface{}) []map[string]interface{} {
	var res []map[string]interface{}
	require.NotNil(t, d)
	b, err := json.Marshal(d)
	require.NoError(t, err)
	require.NotEmpty(t, b)
	err = json.Unmarshal(b, &res)
	require.NoError(t, err)
	return res
}

func assertEvent(T *testing.T, name string, t utc.UTC, dur time.Duration, off int64, n int64, data interface{}, event map[string]interface{}) {
	if name != "" {
		assert.Equal(T, name, event["name"])
	} else if eventName, ok := event["name"].(string); ok {
		assert.Equal(T, name, eventName)
	} else {
		assert.Nil(T, event["name"])
	}
	eventT, err := utc.FromString(event["t"].(string))
	require.NoError(T, err)
	assert.WithinDuration(T, t.Time, eventT.Time, durDelta)
	eventDur, err := duration.FromString(event["dur"].(string))
	require.NoError(T, err)
	assert.InDelta(T, int64(dur), int64(eventDur), float64(durDelta))
	assert.Equal(T, off, int64(event["off"].(float64)))
	assert.Equal(T, n, int64(event["n"].(float64)))
	assert.Equal(T, data, event["data"])
}
