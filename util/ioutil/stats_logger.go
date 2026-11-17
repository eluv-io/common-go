package ioutil

import (
	"encoding/json"
	"io"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/statsutil"
	"github.com/eluv-io/common-go/util/timeutil"
	"github.com/eluv-io/errors-go"
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
//
// Read, Write, Seek, Close may not be called concurrently, but Record may be called concurrently with any other.
//
// Calling Read, Write, or Seek when the wrapped value does not implement the respective interface returns an error. A
// nil limitFn means that no operation is ever reported as slow, and a nil statsLog or slowsLog discards the respective
// log.
//
// Read, Write, and Seek may still be called after Close and pass through to the wrapped value, but they are no longer
// reflected in the logged stats, events, and slow operations.
func NewStatsLogger(
	rwsc interface{}, // Accepts any Reader, Writer, Seeker, Closer
	statsLog func(msg string, fields ...interface{}),
	slowsLog func(msg string, fields ...interface{}),
	start utc.UTC,
	limitFn func(fn int, off int64) time.Duration,
	op string,
	fields ...interface{},
) *StatsLogger {
	noLog := func(string, ...interface{}) {}
	if statsLog == nil {
		statsLog = noLog
	}
	if slowsLog == nil {
		slowsLog = noLog
	}
	if limitFn == nil {
		// no limit: no operation is ever reported as slow
		limitFn = func(int, int64) time.Duration { return math.MaxInt64 }
	}
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

	// mutex guards lastT, lastOff, lastN, events, slows and closed, which are shared between Record and the other
	// methods. It is never held while calling the wrapped reader/writer/seeker/closer or the log functions.
	mutex  sync.Mutex
	closed bool
}

func (l *StatsLogger) Read(p []byte) (int, error) {
	if l.r == nil {
		return 0, errors.E("StatsLogger.Read", errors.K.Invalid,
			"reason", "wrapped value is not an io.Reader", "logger", l.op)
	}
	limit := l.limitFn(ReadFn, l.off)
	l.watch.Reset()
	n, err := l.r.Read(p)
	l.watch.Stop()
	if n > 0 || err != io.EOF || l.watch.Duration() > limit { // Ignore empty reads at EOF, unless slow
		off := l.off
		l.off += int64(n)
		l.stats.update(l.watch.StopTime(), ReadFn, l.watch.Duration(), int64(n))
		l.setLast("read", off, int64(n), limit)
	}
	return n, err
}

func (l *StatsLogger) Write(p []byte) (int, error) {
	if l.w == nil {
		return 0, errors.E("StatsLogger.Write", errors.K.Invalid,
			"reason", "wrapped value is not an io.Writer", "logger", l.op)
	}
	limit := l.limitFn(WriteFn, l.off)
	l.watch.Reset()
	n, err := l.w.Write(p)
	l.watch.Stop()
	if n > 0 || l.watch.Duration() > limit { // Ignore empty writes, unless slow
		off := l.off
		l.off += int64(n)
		l.stats.update(l.watch.StopTime(), WriteFn, l.watch.Duration(), int64(n))
		l.setLast("write", off, int64(n), limit)
	}
	return n, err
}

func (l *StatsLogger) Seek(offset int64, whence int) (int64, error) {
	if l.s == nil {
		return 0, errors.E("StatsLogger.Seek", errors.K.Invalid,
			"reason", "wrapped value is not an io.Seeker", "logger", l.op)
	}
	limit := l.limitFn(SeekFn, l.off)
	l.watch.Reset()
	n, err := l.s.Seek(offset, whence)
	l.watch.Stop()
	pos := n
	if err != nil {
		pos = l.off // a failed seek doesn't move the position, and n is unspecified
	}
	off := l.off
	l.off = pos
	l.stats.update(l.watch.StopTime(), SeekFn, l.watch.Duration(), pos)
	l.setLast("seek", off, pos, limit)
	return n, err
}

func (l *StatsLogger) Close() error {
	var err error
	if l.c != nil {
		err = l.c.Close()
	}
	l.once.Do(func() {
		// log a snapshot: Read/Write/Seek calls after Close must not modify what was handed to the logger
		stats := *l.stats
		stats.Start = l.start
		stats.Duration = duration.Spec(utc.Since(l.start))

		// take ownership of the events: Record calls after this point are dropped
		l.mutex.Lock()
		l.closed = true
		events, slows := l.events, l.slows
		l.events, l.slows = nil, nil
		l.mutex.Unlock()

		op, limit := l.normalize(slows)
		msg := op + " stats"
		fields := append(slices.Clone(l.fields), "stats", &stats, "events", events)
		l.statsLog(msg, fields...)
		if len(slows) > 0 {
			msg := op + " slow operations"
			fields := slices.Clone(l.fields)
			if limit > 0 {
				fields = append(fields, "limit", limit)
			}
			fields = append(fields, "ops", slows)
			l.slowsLog(msg, fields...)
		}
	})
	return err
}

// Record accepts a custom now timestamp for the recorded event time; otherwise, the event time will be the timestamp
// of the last Read/Write/Seek call result.
//
// Record may be called concurrently with any other method. Events recorded after Close are dropped.
func (l *StatsLogger) Record(name string, data interface{}, now ...utc.UTC) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.closed {
		return
	}
	t := l.lastT
	if len(now) > 0 {
		t = now[0]
	}
	l.events = append(l.events, &statsEvent{
		T:    t,
		Name: name,
		Dur:  duration.Spec(t.Sub(l.start)),
		Off:  l.lastOff,
		N:    l.lastN,
		Data: data,
	})
}

// setLast stores the result of the latest Read/Write/Seek, which is attached to events passed to Record. If the call
// exceeded the given limit, it also records a slow event. Everything happens under a single lock, so Record always
// sees the time, offset and result of one and the same call.
func (l *StatsLogger) setLast(name string, off int64, n int64, limit time.Duration) {
	t, dur := l.watch.StopTime(), l.watch.Duration()
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.lastT, l.lastOff, l.lastN = t, off, n
	if dur > limit && !l.closed {
		l.slows = append(l.slows, &statsEvent{
			T:     t,
			Name:  name,
			Limit: duration.Spec(limit),
			Dur:   duration.Spec(dur),
			Off:   off,
			N:     n,
		})
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
func (l *StatsLogger) normalize(slows []*statsEvent) (string, duration.Spec) {
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
	for _, s := range slows {
		limits[s.Limit]++
		if limit == 0 || limits[s.Limit] > limits[limit] {
			limit = s.Limit
		}
	}
	if limits[limit] <= len(slows)/2 {
		limit = 0
	}

	if name != "" || limit > 0 {
		for _, s := range slows {
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
