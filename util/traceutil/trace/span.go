package trace

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/eluv-io/utc-go"

	"github.com/eluv-io/common-go/format/duration"
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
}

type Event struct {
	Name string                 `json:"name"`
	Time utc.UTC                `json:"-"`
	Attr map[string]interface{} `json:"attr,omitempty"`
}

// ---------------------------------------------------------------------------------------------------------------------

type NoopSpan struct{}

func (n NoopSpan) Start(ctx context.Context, _ string) (context.Context, Span) {
	return ctx, n
}

func (n NoopSpan) End()                                                 {}
func (n NoopSpan) Attribute(name string, val interface{})               {}
func (n NoopSpan) Event(name string, attributes map[string]interface{}) {}
func (n NoopSpan) IsRecording() bool                                    { return false }
func (n NoopSpan) Json() string                                         { return "" }
func (n NoopSpan) Attributes() map[string]interface{}                   { return nil }
func (n NoopSpan) Events() []*Event                                     { return nil }
func (n NoopSpan) FindByName(string) Span                               { return nil }
func (n NoopSpan) StartTime() utc.UTC                                   { return utc.Zero }
func (n NoopSpan) EndTime() utc.UTC                                     { return utc.Zero }
func (n NoopSpan) Duration() time.Duration                              { return 0 }
func (n NoopSpan) MaxDuration() time.Duration                           { return 0 }
func (n NoopSpan) MarshalExtended() bool                                { return false }
func (n NoopSpan) SetMarshalExtended()                                  {}
func (n NoopSpan) SlowCutoff() time.Duration                            { return 0 }
func (n NoopSpan) SetSlowCutoff(cutoff time.Duration)                   {}

// ---------------------------------------------------------------------------------------------------------------------

func newSpan(name string) *RecordingSpan {
	s := &RecordingSpan{
		startTime: utc.Now(),
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
	cutoff    time.Duration
	extended  bool
}

type recordingData struct {
	Name     string                 `json:"name"`
	Duration duration.Spec          `json:"time"`
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
	sub := newSpan(name)
	sub.Parent = s

	s.mutex.Lock()
	s.Data.Subs = append(s.Data.Subs, sub)
	s.mutex.Unlock()

	return ContextWithSpan(ctx, sub), sub
}

func (s *RecordingSpan) End() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.endTime != utc.Zero {
		return
	}
	s.endTime = utc.Now()
	s.duration = s.endTime.Sub(s.startTime)
	s.Data.Duration = duration.Spec(s.duration).RoundTo(1)
	s.Data.End = s.endTime.String()
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
		Time: utc.Now(),
		Attr: attributes,
	})
}

func (s *RecordingSpan) IsRecording() bool {
	return true
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

	s.cutoff = cutoff
}

func (s *RecordingSpan) SlowCutoff() time.Duration {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.cutoff
}

func (s *RecordingSpan) MarshalJSON() ([]byte, error) {
	if s.MarshalExtended() {
		return json.Marshal(s.Data)
	} else {
		return json.Marshal(s.Data.recordingData)
	}
}
