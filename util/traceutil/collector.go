package traceutil

import (
	"encoding/json"
	"sync"

	sdktrace "go.opentelemetry.io/otel/api/trace"
	"go.opentelemetry.io/otel/sdk/export/trace"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc/codes"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/duration"
	"github.com/qluvio/content-fabric/format/utc"
	"github.com/qluvio/content-fabric/util/jsonutil"
)

type TraceCollector struct {
	mutex  sync.Mutex
	traces map[sdktrace.ID]*TraceInfo
}

func (e *TraceCollector) AddSpan(span sdktrace.Span, data interface{}) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if e.traces == nil {
		e.traces = map[sdktrace.ID]*TraceInfo{}
	}

	if _, found := e.traces[span.SpanContext().TraceID]; found {
		// trace is already registered...
		return
	}

	e.traces[span.SpanContext().TraceID] = &TraceInfo{
		ID:         span.SpanContext().TraceID,
		RootSpanID: span.SpanContext().SpanID,
		Data:       data,
		Spans:      map[sdktrace.SpanID]*SpanInfo{},
	}
}

func (e *TraceCollector) SpanEnded(data *trace.SpanData) (trc *TraceInfo, completed bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	var ok bool
	trc, ok = e.traces[data.SpanContext.TraceID]
	if !ok {
		log.Warn("TraceCollector: trace not found", "span", jsonutil.MarshalString(data))
		return nil, false
	}

	trc.AddSpan(data)

	if trc.RootSpanID != data.SpanContext.SpanID {
		return nil, false
	}

	delete(e.traces, data.SpanContext.TraceID)

	return trc, true
}

type TraceInfo struct {
	ID             sdktrace.ID
	RootSpanID     sdktrace.SpanID
	Spans          map[sdktrace.SpanID]*SpanInfo
	Data           interface{} // additional arbitrary data associated with the trace
	marshalMinimal bool
}

func (t *TraceInfo) AddSpan(span *trace.SpanData) {
	info, found := t.Spans[span.SpanContext.SpanID]
	if !found {
		info = &SpanInfo{SpanData: span, Trace: t}
		t.Spans[span.SpanContext.SpanID] = info
	} else {
		info.SpanData = span
	}
	if span.ParentSpanID.IsValid() {
		parent, ok := t.Spans[span.ParentSpanID]
		if !ok {
			parent = &SpanInfo{Trace: t, Children: []*SpanInfo{info}}
			t.Spans[span.ParentSpanID] = parent
		} else {
			parent.Children = append(parent.Children, info)
		}
	}
}

func (t *TraceInfo) String() string {
	bytes, err := json.Marshal(t)
	if err != nil {
		return errors.E("failed to marshal trace", err).Error()
	}
	return string(bytes)
}

func (t *TraceInfo) MinimalString() string {
	defer func(prev bool) {
		t.marshalMinimal = prev
	}(t.marshalMinimal)

	t.marshalMinimal = true
	return t.String()
}

func (t *TraceInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.Spans[t.RootSpanID])
}

func (t *TraceInfo) RootSpan() *SpanInfo {
	return t.Spans[t.RootSpanID]
}

func (t *TraceInfo) FindSpanByName(s string) *SpanInfo {
	for _, spanInfo := range t.Spans {
		if spanInfo.Name == s {
			return spanInfo
		}
	}
	return nil
}

type SpanInfo struct {
	*trace.SpanData
	Trace    *TraceInfo
	Children []*SpanInfo
}

func (s *SpanInfo) MarshalJSON() ([]byte, error) {
	type data struct {
		TraceID      string            `json:"trace_id,omitempty"`
		SpanID       string            `json:"span_id,omitempty"`
		ParentSpanID string            `json:"parent_span_id,omitempty"`
		SpanKind     sdktrace.SpanKind `json:"span_kind,omitempty"`
		Name         string            `json:"name,omitempty"`
		StartTime    *utc.UTC          `json:"start_time,omitempty"`
		// EndTime                  time.Time
		Duration                 duration.Spec          `json:"time"`
		Attributes               map[string]interface{} `json:"attr,omitempty"`
		MessageEvents            []trace.Event          `json:"message_events,omitempty"`
		Links                    []sdktrace.Link        `json:"links,omitempty"`
		StatusCode               codes.Code             `json:"status_code,omitempty"`
		StatusMessage            string                 `json:"status_message,omitempty"`
		HasRemoteParent          bool                   `json:"has_remote_parent,omitempty"`
		DroppedAttributeCount    int                    `json:"dropped_attribute_count,omitempty"`
		DroppedMessageEventCount int                    `json:"dropped_message_event_count,omitempty"`
		DroppedLinkCount         int                    `json:"dropped_link_count,omitempty"`
		ChildSpanCount           int                    `json:"child_span_count,omitempty"`
		Resource                 *resource.Resource     `json:"resource,omitempty"`
		Children                 []*SpanInfo            `json:"subs,omitempty"`
	}

	out := &data{
		Name:                     s.Name,
		Duration:                 duration.Spec(s.EndTime.Sub(s.StartTime)).RoundTo(1),
		Attributes:               s.SimplifiedAttributes(),
		MessageEvents:            s.MessageEvents,
		Links:                    s.Links,
		StatusCode:               s.StatusCode,
		StatusMessage:            s.StatusMessage,
		HasRemoteParent:          s.HasRemoteParent,
		DroppedAttributeCount:    s.DroppedAttributeCount,
		DroppedMessageEventCount: s.DroppedMessageEventCount,
		DroppedLinkCount:         s.DroppedLinkCount,
		Resource:                 s.Resource,
		Children:                 s.Children,
	}

	if !s.Trace.marshalMinimal {
		out.SpanID = s.SpanContext.SpanID.String()
		out.SpanKind = s.SpanKind
		out.StartTime = &utc.UTC{Time: s.StartTime}
		out.ChildSpanCount = s.ChildSpanCount

		if s.ParentSpanID.IsValid() {
			out.ParentSpanID = s.ParentSpanID.String()
		} else {
			out.TraceID = s.SpanContext.TraceID.String()
		}
	}

	return json.Marshal(out)
}

func (s *SpanInfo) SimplifiedAttributes() map[string]interface{} {
	if len(s.Attributes) == 0 {
		return nil
	}
	res := map[string]interface{}{}
	for _, attribute := range s.Attributes {
		res[string(attribute.Key)] = attribute.Value.AsInterface()
	}
	return res
}
