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

// StatsLogger is a wrapper around a Reader/Writer/Seeker/Closer (or any combination thereof) that collects stats for
// Read/Write/Seek calls and custom events specified by the caller. It will track stats (count, min, max, sum, mean) for
// call durations and report any call durations slower than the given limit. At the first Close call, the aggregated
// stats, collected events, and slow operations will be logged using the given loggers.
//
// For each custom event specified by the caller, a name and optional data are given by the caller, and StatsLogger
// collects the event along with the time, offset, and result of the last Read/Write/Seek call before the event.
type StatsLogger struct {
	r        io.Reader
	w        io.Writer
	s        io.Seeker
	c        io.Closer
	statsLog func(msg string, fields ...interface{})
	slowsLog func(msg string, fields ...interface{})
	start    utc.UTC
	limit    time.Duration
	op       string
	fields   []interface{}
	off      int64
	lastT    utc.UTC
	lastOff  int64
	lastN    int64
	stats    statsutil.Statistics[time.Duration]
	events   []*statsEvent
	slows    []*statsEvent
	once     sync.Once
}

func NewStatsLogger(
	rwsc interface{}, // Accepts any Reader, Writer, Seeker, Closer
	statsLog func(msg string, fields ...interface{}),
	slowsLog func(msg string, fields ...interface{}),
	start utc.UTC,
	limit time.Duration,
	op string,
	fields ...interface{},
) *StatsLogger {
	l := &StatsLogger{statsLog: statsLog, slowsLog: slowsLog, start: start, limit: limit, op: op, fields: fields}
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
	return l
}

func (l *StatsLogger) Read(p []byte) (int, error) {
	if l.r == nil {
		panic(l.op + ": StatsLogger.Read called but missing reader")
	}
	watch := timeutil.StartWatch()
	n, err := l.r.Read(p)
	watch.Stop()
	if n > 0 || err != io.EOF || watch.Duration() > l.limit { // Ignore empty reads at EOF, unless slow
		l.lastT = watch.StopTime()
		l.lastN = int64(n)
		l.lastOff = l.off
		l.off += l.lastN
		l.stats.Update(l.lastT, watch.Duration())
		if watch.Duration() > l.limit {
			l.record(true, "Read", watch.Duration(), nil)
		}
	}
	return n, err
}

func (l *StatsLogger) Write(p []byte) (int, error) {
	if l.w == nil {
		panic(l.op + ": StatsLogger.Write called but missing writer")
	}
	watch := timeutil.StartWatch()
	n, err := l.w.Write(p)
	watch.Stop()
	if n > 0 || watch.Duration() > l.limit { // Ignore empty writes, unless slow
		l.lastT = watch.StopTime()
		l.lastN = int64(n)
		l.lastOff = l.off
		l.off += l.lastN
		l.stats.Update(l.lastT, watch.Duration())
		if watch.Duration() > l.limit {
			l.record(true, "Write", watch.Duration(), nil)
		}
	}
	return n, err
}

func (l *StatsLogger) Seek(offset int64, whence int) (int64, error) {
	if l.s == nil {
		panic(l.op + ": StatsLogger.Seek called but missing seeker")
	}
	watch := timeutil.StartWatch()
	n, err := l.s.Seek(offset, whence)
	watch.Stop()
	l.lastT = watch.StopTime()
	l.lastN = n
	l.lastOff = l.off
	l.off = l.lastN
	l.stats.Update(l.lastT, watch.Duration())
	if watch.Duration() > l.limit {
		l.record(true, "Seek", watch.Duration(), nil)
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
		msg := l.op + " stats"
		fields := append(l.fields, "stats", l.stats, "events", l.events)
		l.statsLog(msg, fields...)
		if len(l.slows) > 0 {
			msg := clean(l.slows, l.op) + " slow operations"
			fields := append(l.fields, "limit", l.limit, "ops", l.slows)
			l.slowsLog(msg, fields...)
		}
	})
	return err
}

func (l *StatsLogger) Record(name string, data interface{}) {
	l.record(false, name, l.lastT.Sub(l.start), data)
}

func (l *StatsLogger) record(slow bool, name string, dur time.Duration, data interface{}) {
	s := &statsEvent{Name: name, T: l.lastT, Dur: duration.Spec(dur), Off: l.lastOff, N: l.lastN, Data: data}
	if slow {
		l.slows = append(l.slows, s)
	} else {
		l.events = append(l.events, s)
	}
}

type statsEvent struct {
	Name string        `json:"name,omitempty"`
	T    utc.UTC       `json:"t"`
	Dur  duration.Spec `json:"dur"`
	Off  int64         `json:"off"`
	N    int64         `json:"n"`
	Data interface{}   `json:"data,omitempty"`
}

func (e *statsEvent) String() string {
	b, _ := json.Marshal(e)
	return string(b)
}

// clean clears all names if all names are the same and returns "OP.NAME"; otherwise, does nothing and returns "OP".
func clean(events []*statsEvent, op string) string {
	name := ""
	for _, e := range events {
		if name == "" {
			name = e.Name
		} else if e.Name != name {
			return op
		}
	}
	for _, e := range events {
		e.Name = ""
	}
	return op + "." + name
}
