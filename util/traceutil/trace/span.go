package trace

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/utc-go"
)

// Span represents an operation that is named and timed. It may contain sub-spans, representing nested sub-operations. A
// span can also record an arbitrary number of attributes (key-value pairs) and events (bundled attributes with a name
// and timestamp)
type Span interface {
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

// ---------------------------------------------------------------------------------------------------------------------

func newSpan(name string) *RecordingSpan {
	s := &RecordingSpan{
		StartTime: utc.Now(),
	}
	s.Data.Name = name
	return s
}

type RecordingSpan struct {
	mutex     sync.Mutex
	Parent    Span
	StartTime utc.UTC
	EndTime   utc.UTC
	Data      struct {
		Name     string                 `json:"name"`
		Duration duration.Spec          `json:"time"`
		Attr     map[string]interface{} `json:"attr,omitempty"`
		Events   []*Event               `json:"evnt,omitempty"`
		Subs     []Span                 `json:"subs,omitempty"`
	}
}

func (s *RecordingSpan) Start(ctx context.Context, name string) (context.Context, Span) {
	sub := newSpan(name)

	s.mutex.Lock()
	s.Data.Subs = append(s.Data.Subs, sub)
	s.mutex.Unlock()

	return ContextWithSpan(ctx, sub), sub
}

func (s *RecordingSpan) End() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.EndTime != utc.Zero {
		return
	}
	s.EndTime = utc.Now()
	s.Data.Duration = duration.Spec(s.EndTime.Sub(s.StartTime)).RoundTo(1)
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

func (s *RecordingSpan) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Data)
}
