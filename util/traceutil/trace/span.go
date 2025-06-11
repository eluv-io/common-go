package trace

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/eluv-io/utc-go"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/sliceutil"
)

// Span represents an operation that is named and timed. It may contain sub-spans, representing nested sub-operations. A
// span can also record an arbitrary number of attributes (key-value pairs) and events (bundled attributes with a name
// and timestamp)
type Span interface {
	ExtendedSpan

	// End completes the span. No updates are allowed to span after it
	// ends. The only exception is setting status of the span.
	End()

	// Attribute sets an arbitrary attribute.
	Attribute(name string, val interface{})

	// Event adds an event with the given name and attributes.
	Event(name string, attributes map[string]interface{})

	// IsRecording returns true if the span is active and recording events is enabled.
	IsRecording() bool

	// SlowOnly returns true if the span is active and is only for slow request tracing.
	SlowOnly() bool

	// Json converts the span to its JSON representation.
	Json() string

	// Start creates and starts a sub-span. The returned context holds a reference to the sub-span and may be retrieved
	// with SpanFromContext.
	Start(ctx context.Context, name string) (context.Context, Span)

	// Attributes returns the span's attribute set.
	Attributes() map[string]interface{}

	// Events returns the span's attribute set.
	Events() []*Event

	// FindByName returns the first sub span with the given name (depth-first) or nil if not found.
	FindByName(name string) Span
}

type ExtendedSpan interface {
	// StartTime returns the start time of the span.
	StartTime() utc.UTC

	// EndTime returns the end time of the span.
	EndTime() utc.UTC

	// Duration returns the duration of the span.
	Duration() time.Duration

	// MaxDuration returns the maximum duration of the span and of all sub-spans.
	MaxDuration() time.Duration

	// MarshalExtended returns true if the span is set to use an extended JSON representation during marshaling.
	MarshalExtended() bool

	// SetMarshalExtended sets the span to use an extended JSON representation during marshaling.
	SetMarshalExtended()

	// SetSlowCutoff sets the slow cutoff duration for the span. A span that exceeds this duration
	// will be considered slow.
	SetSlowCutoff(cutoff time.Duration)

	// SlowCutoff is the slow cutoff duration for the span. A span that exceeds its slow cutoff is
	// considered an ususually slow operation.
	SlowCutoff() time.Duration

	// MarshalSlowOnly returns the marshalling of all spans for which a sub-span or parent span is
	// slower than the cutoff.
	MarshalSlowOnly() ([]byte, error, bool)

	// FindAncestorByAttr returns the ancestor span (or self) that contains the given attribute. Returns a noop span if
	// not found.
	FindAncestorByAttr(name string, value any) Span
}

type Event struct {
	Name string                 `json:"name"`
	At   duration.Spec          `json:"at"`
	Attr map[string]interface{} `json:"attr,omitempty"`
}

// ---------------------------------------------------------------------------------------------------------------------

type NoopSpan struct{}

func (n NoopSpan) Start(ctx context.Context, _ string) (context.Context, Span) {
	return ctx, n
}

func (n NoopSpan) End()                                     {}
func (n NoopSpan) Attribute(_ string, _ interface{})        {}
func (n NoopSpan) Event(_ string, _ map[string]interface{}) {}
func (n NoopSpan) IsRecording() bool                        { return false }
func (n NoopSpan) SlowOnly() bool                           { return false }
func (n NoopSpan) Json() string                             { return "" }
func (n NoopSpan) Attributes() map[string]interface{}       { return nil }
func (n NoopSpan) Events() []*Event                         { return nil }
func (n NoopSpan) FindByName(string) Span                   { return nil }
func (n NoopSpan) StartTime() utc.UTC                       { return utc.Zero }
func (n NoopSpan) EndTime() utc.UTC                         { return utc.Zero }
func (n NoopSpan) Duration() time.Duration                  { return 0 }
func (n NoopSpan) MaxDuration() time.Duration               { return 0 }
func (n NoopSpan) MarshalExtended() bool                    { return false }
func (n NoopSpan) SetMarshalExtended()                      {}
func (n NoopSpan) SlowCutoff() time.Duration                { return 0 }
func (n NoopSpan) SetSlowCutoff(_ time.Duration)            {}
func (n NoopSpan) MarshalSlowOnly() ([]byte, error, bool)   { return nil, nil, false }
func (n NoopSpan) FindAncestorByAttr(_ string, _ any) Span  { return n }

// ---------------------------------------------------------------------------------------------------------------------

func newSpan(name string, slowOnly bool) *RecordingSpan {
	s := &RecordingSpan{
		startTime: utc.Now(),
		slowOnly:  slowOnly,
	}
	s.Data.Name = name
	s.Data.Start = s.startTime.String()
	return s
}

type RecordingSpan struct {
	mutex     sync.Mutex
	Parent    Span
	Data      recordingExtendedData
	startTime utc.UTC
	endTime   utc.UTC
	duration  time.Duration
	// extended must not be protected by a lock, as child spans may look up to their parent span to
	// determine marshalling behavior
	extended bool
	slowOnly bool
}

type recordingData struct {
	Name     string                 `json:"name"`
	Duration duration.Spec          `json:"time"`
	Cutoff   duration.Spec          `json:"cutoff,omitempty"`
	Attr     map[string]interface{} `json:"attr,omitempty"`
	Events   []*Event               `json:"evnt,omitempty"`
	Subs     []Span                 `json:"subs,omitempty"`
}

type recordingExtendedData struct {
	recordingData
	Start string `json:"start"`
	End   string `json:"end"`
}

func (s *RecordingSpan) Start(ctx context.Context, name string) (context.Context, Span) {
	sub := newSpan(name, s.slowOnly)
	sub.Parent = s

	s.mutex.Lock()
	s.Data.Subs = append(s.Data.Subs, sub)
	s.mutex.Unlock()

	return ContextWithSpan(ctx, sub), sub
}

func (s *RecordingSpan) End() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.setEnd()
}

// setEnd assumes that the lock is held
func (s *RecordingSpan) setEnd() {
	if s.endTime != utc.Zero {
		return
	}
	s.endTime = utc.Now()
	s.duration = s.endTime.Sub(s.startTime)
	s.Data.Duration = duration.Spec(s.duration).RoundTo(1)
	s.Data.End = s.endTime.String()
}

func (s *RecordingSpan) undoEnd() {
	s.endTime = utc.Zero
	s.duration = 0
	s.Data.Duration = 0
	s.Data.End = ""
}

func (s *RecordingSpan) Attribute(name string, val interface{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.Data.Attr == nil {
		s.Data.Attr = make(map[string]interface{})
	}
	s.Data.Attr[name] = val
}

func (s *RecordingSpan) Event(name string, attributes map[string]interface{}) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.Data.Events = append(s.Data.Events, &Event{
		Name: name,
		At:   duration.Spec(utc.Now().Sub(s.startTime)),
		Attr: attributes,
	})
}

func (s *RecordingSpan) IsRecording() bool {
	return true
}

func (s *RecordingSpan) SlowOnly() bool {
	return s.slowOnly
}

func (s *RecordingSpan) Json() string {
	res, err := s.MarshalJSON()
	if err != nil {
		return "failed to marshal span: " + err.Error()
	}
	return string(res)
}

func (s *RecordingSpan) Attributes() map[string]interface{} {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.Data.Attr
}

func (s *RecordingSpan) Events() []*Event {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.Data.Events
}

func (s *RecordingSpan) FindByName(name string) Span {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.Data.Name == name {
		return s
	}
	for _, sub := range s.Data.Subs {
		res := sub.FindByName(name)
		if res != nil {
			return res
		}
	}
	return nil
}

func (s *RecordingSpan) StartTime() utc.UTC {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.startTime
}

func (s *RecordingSpan) EndTime() utc.UTC {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.endTime
}

func (s *RecordingSpan) Duration() time.Duration {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.duration
}

func (s *RecordingSpan) MaxDuration() time.Duration {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	d := s.duration
	for _, ss := range s.Data.Subs {
		sd := ss.MaxDuration()
		if sd > d {
			d = sd
		}
	}
	return d
}

func (s *RecordingSpan) MarshalExtended() bool {
	if s.extended {
		return true
	} else if s.Parent != nil {
		// Check parent for extended flag
		return s.Parent.MarshalExtended()
	}
	return false
}

func (s *RecordingSpan) SetMarshalExtended() {
	s.extended = true
}

func (s *RecordingSpan) SetSlowCutoff(cutoff time.Duration) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.Data.Cutoff = duration.Spec(cutoff).RoundTo(1)
}

func (s *RecordingSpan) SlowCutoff() time.Duration {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.Data.Cutoff.Duration()
}

func (s *RecordingSpan) slowSpans() (Span, bool) {
	notEnded := s.endTime == utc.Zero
	if notEnded {
		s.setEnd()
		defer s.undoEnd()
	}
	sCopy := s.copy()

	if s.Data.Cutoff.Duration() != time.Duration(0) && s.duration > s.Data.Cutoff.Duration() {
		return s, true
	}

	sawSlow := false
	subsCopy := sliceutil.Copy(s.Data.Subs)
	sCopy.Data.Subs = []Span{}
	for _, sub := range subsCopy {
		subC, slow := sub.(*RecordingSpan).slowSpans()
		sawSlow = sawSlow || slow
		if slow {
			sCopy.Data.Subs = append(sCopy.Data.Subs, subC)
		}
	}

	return sCopy, sawSlow
}

func (s *RecordingSpan) MarshalJSON() ([]byte, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	notEnded := s.endTime == utc.Zero
	if notEnded {
		s.setEnd()
		defer s.undoEnd()
	}

	var ret []byte
	var err error
	if s.MarshalExtended() {
		ret, err = json.Marshal(s.Data)
	} else {
		ret, err = json.Marshal(s.Data.recordingData)
	}
	return ret, err
}

func (s *RecordingSpan) MarshalSlowOnly() ([]byte, error, bool) {
	s.mutex.Lock()
	slowSpans, sawSlow := s.slowSpans()
	s.mutex.Unlock() // release lock so that it doesn't deadlock with MarshalJSON in case this span is also a slow span!
	if !sawSlow {
		return nil, nil, false
	}
	d, err := json.Marshal(slowSpans)
	return d, err, true
}

// copy returns a shallow copy of a recording span. The lock must be held while this function is
// called. In particular, the referenced elements from within the data (Attr, Events, Subs), cannot
// be modified.
func (s *RecordingSpan) copy() *RecordingSpan {
	c := &RecordingSpan{
		Parent:    s.Parent,
		Data:      s.Data,
		startTime: s.startTime,
		endTime:   s.endTime,
		duration:  s.duration,
		extended:  s.extended,
		slowOnly:  s.slowOnly,
	}
	return c
}

func (s *RecordingSpan) FindAncestorByAttr(name string, value any) Span {
	a := s
	for a != nil {
		if v, ok := s.attr(name, a); ok && v == value {
			return a
		}
		a, _ = a.Parent.(*RecordingSpan)
	}
	return NoopSpan{}
}

func (s *RecordingSpan) attr(name string, a *RecordingSpan) (any, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	v, ok := a.Data.Attr[name]
	return v, ok
}
