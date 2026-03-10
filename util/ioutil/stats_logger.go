package ioutil

import (
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/statsutil"
	"github.com/eluv-io/common-go/util/timeutil"
	"github.com/eluv-io/utc-go"
)

const (
	ReadFn = iota
	WriteFn
	SeekFn
)

// NewStatsLogger returns a wrapper around a Reader/Writer/Seeker/Closer (or any combination thereof) that collects
// stats for Read/Write/Seek calls and custom events specified by the caller. It will track stats (count, min, max,
// sum, mean) for call durations and report any call durations slower than the given limit. At the first Close call,
// the aggregated stats, collected events, and slow operations will be logged using the given loggers.
//
// For each custom event specified by the caller, a name and optional data are given by the caller, and StatsLogger
// collects the event along with the time, offset, and result of the last Read/Write/Seek call before the event.
func NewStatsLogger(
	rwsc interface{}, // Accepts any Reader, Writer, Seeker, Closer
	statsLog func(msg string, fields ...interface{}),
	slowsLog func(msg string, fields ...interface{}),
	start utc.UTC,
	limitFn func(fn int, off int64) time.Duration,
	op string,
	fields ...interface{},
) *StatsLogger {
	l := &StatsLogger{statsLog: statsLog, slowsLog: slowsLog, start: start, limitFn: limitFn, op: op, fields: fields}
	if r, ok := rwsc.(io.Reader); ok {
		l.r = r
	}
	if w, ok := rwsc.(io.Writer); ok {
		l.w = w
	}
	if s, ok := rwsc.(io.Seeker); ok {
		l.s = s
	}
	if c, ok := rwsc.(io.Closer); ok {
		l.c = c
	}
	l.lastT = start
	l.watch = timeutil.StartWatch()
	l.stats = &statistics{}
	return l
}

type StatsLogger struct {
	r        io.Reader
	w        io.Writer
	s        io.Seeker
	c        io.Closer
	statsLog func(msg string, fields ...interface{})
	slowsLog func(msg string, fields ...interface{})
	start    utc.UTC
	limitFn  func(fn int, off int64) time.Duration
	op       string
	fields   []interface{}
	off      int64
	lastT    utc.UTC
	lastOff  int64
	lastN    int64
	watch    *timeutil.StopWatch
	stats    *statistics
	events   []*statsEvent
	slows    []*statsEvent
	once     sync.Once
}

func (l *StatsLogger) Read(p []byte) (int, error) {
	if l.r == nil {
		panic(l.op + ": StatsLogger.Read called but missing reader")
	}
	limit := l.limitFn(ReadFn, l.off)
	l.watch.Reset()
	n, err := l.r.Read(p)
	l.watch.Stop()
	if n > 0 || err != io.EOF || l.watch.Duration() > limit { // Ignore empty reads at EOF, unless slow
		l.lastT = l.watch.StopTime()
		l.lastN = int64(n)
		l.lastOff = l.off
		l.off += l.lastN
		l.stats.update(l.lastT, ReadFn, l.watch.Duration(), int64(n))
		if l.watch.Duration() > limit {
			l.record(true, l.watch.StopTime(), "read", limit, l.watch.Duration(), nil)
		}
	}
	return n, err
}

func (l *StatsLogger) Write(p []byte) (int, error) {
	if l.w == nil {
		panic(l.op + ": StatsLogger.Write called but missing writer")
	}
	limit := l.limitFn(WriteFn, l.off)
	l.watch.Reset()
	n, err := l.w.Write(p)
	l.watch.Stop()
	if n > 0 || l.watch.Duration() > limit { // Ignore empty writes, unless slow
		l.lastT = l.watch.StopTime()
		l.lastN = int64(n)
		l.lastOff = l.off
		l.off += l.lastN
		l.stats.update(l.lastT, WriteFn, l.watch.Duration(), int64(n))
		if l.watch.Duration() > limit {
			l.record(true, l.watch.StopTime(), "write", limit, l.watch.Duration(), nil)
		}
	}
	return n, err
}

func (l *StatsLogger) Seek(offset int64, whence int) (int64, error) {
	if l.s == nil {
		panic(l.op + ": StatsLogger.Seek called but missing seeker")
	}
	limit := l.limitFn(SeekFn, l.off)
	l.watch.Reset()
	n, err := l.s.Seek(offset, whence)
	l.watch.Stop()
	l.lastT = l.watch.StopTime()
	l.lastN = n
	l.lastOff = l.off
	l.off = l.lastN
	l.stats.update(l.lastT, SeekFn, l.watch.Duration(), n)
	if l.watch.Duration() > limit {
		l.record(true, l.watch.StopTime(), "seek", limit, l.watch.Duration(), nil)
	}
	return n, err
}

func (l *StatsLogger) Close() error {
	var err error
	if l.c != nil {
		err = l.c.Close()
	}
	l.once.Do(func() {
		l.stats.Start = l.start
		l.stats.Duration = duration.Spec(utc.Since(l.start))
		l.stats.Mean = l.stats.Mean / float64(time.Millisecond) // Express mean in milliseconds, not nanoseconds
		op, limit := l.normalize()
		msg := op + " stats"
		fields := append(l.fields, "stats", l.stats, "events", l.events)
		l.statsLog(msg, fields...)
		if len(l.slows) > 0 {
			msg := op + " slow operations"
			fields := l.fields
			if limit > 0 {
				fields = append(fields, "limit", limit)
			}
			fields = append(fields, "ops", l.slows)
			l.slowsLog(msg, fields...)
		}
	})
	return err
}

// Record accepts a custom now timestamp for the recorded event time; otherwise, the event time will be the timestamp
// of the last Read/Write/Seek call result.
func (l *StatsLogger) Record(name string, data interface{}, now ...utc.UTC) {
	t := l.lastT
	if len(now) > 0 {
		t = now[0]
	}
	l.record(false, t, name, 0, t.Sub(l.start), data)
}

func (l *StatsLogger) record(slow bool, t utc.UTC, name string, limit time.Duration, dur time.Duration, data interface{}) {
	s := &statsEvent{T: t, Name: name, Limit: duration.Spec(limit), Dur: duration.Spec(dur), Off: l.lastOff, N: l.lastN, Data: data}
	if slow {
		l.slows = append(l.slows, s)
	} else {
		l.events = append(l.events, s)
	}
}

type statistics struct {
	statsutil.Statistics[duration.Spec]
	Reads      int   `json:"reads,omitempty"`
	Writes     int   `json:"writes,omitempty"`
	Seeks      int   `json:"seeks,omitempty"`
	ReadBytes  int64 `json:"rbytes,omitempty"`
	WriteBytes int64 `json:"wbytes,omitempty"`
	SeekBytes  int64 `json:"sbytes,omitempty"`
}

func (s *statistics) update(now utc.UTC, fn int, val time.Duration, bytes ...int64) {
	s.Update(now, duration.Spec(val))
	b := int64(0)
	if len(bytes) > 0 {
		b = bytes[0]
	}
	switch fn {
	case ReadFn:
		s.Reads++
		s.ReadBytes += b
	case WriteFn:
		s.Writes++
		s.WriteBytes += b
	case SeekFn:
		s.Seeks++
		s.SeekBytes = b
	}
}

func (s *statistics) String() string {
	b, _ := json.Marshal(s)
	return string(b)
}

type statsEvent struct {
	T     utc.UTC       `json:"t"`
	Name  string        `json:"name,omitempty"`
	Limit duration.Spec `json:"limit,omitempty"`
	Dur   duration.Spec `json:"dur"`
	Off   int64         `json:"off"`
	N     int64         `json:"n"`
	Data  interface{}   `json:"data,omitempty"`
}

func (e *statsEvent) String() string {
	b, _ := json.Marshal(e)
	return string(b)
}

// normalize determines if all calls made through StatsLogger were the same and, if so, removes all slow event names.
// Additionally, if a majority of slow event limits are the same, it clears all limits equal to the majority limit.
// Returns "OP.NAME" (if all calls were the same; otherwise "OP") and the majority limit (if applicable).
func (l *StatsLogger) normalize() (string, duration.Spec) {
	var name string
	if l.stats.Reads > 0 && l.stats.Writes == 0 && l.stats.Seeks == 0 {
		name = "Read"
	} else if l.stats.Reads == 0 && l.stats.Writes > 0 && l.stats.Seeks == 0 {
		name = "Write"
	} else if l.stats.Reads == 0 && l.stats.Writes == 0 && l.stats.Seeks > 0 {
		name = "Seek"
	}

	var limit duration.Spec
	limits := make(map[duration.Spec]int)
	for _, s := range l.slows {
		limits[s.Limit]++
		if limit == 0 || limits[s.Limit] > limits[limit] {
			limit = s.Limit
		}
	}
	if limits[limit] <= len(l.slows)/2 {
		limit = 0
	}

	if name != "" || limit > 0 {
		for _, s := range l.slows {
			if name != "" {
				s.Name = ""
			}
			if s.Limit == limit {
				s.Limit = 0
			}
		}
	}

	if name == "" {
		return l.op, limit
	} else {
		return l.op + "." + name, limit
	}
}
