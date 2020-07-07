package stackutil

import (
	"bytes"
	"io/ioutil"
	stdlog "log"
	"sort"

	"github.com/maruel/panicparse/stack"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/util/numberutil"
)

const (
	Normal     = stack.AnyPointer
	Aggressive = stack.AnyValue
)

// An aggregated stack dump - a wrapper around the "buckets" of the 3rd-party
// panicparse library.
type AggregateStack struct {
	trace      string
	buckets    []*stack.Bucket
	similarity stack.Similarity
}

// Aggregate creates an aggregated stack dump object from the given stack trace.
// The aggregation will be performed more or less aggressively based on the
// provided similarity parameter.
func Aggregate(trace string, sim stack.Similarity) (*AggregateStack, error) {
	in := bytes.NewBuffer([]byte(trace))

	c, err := stack.ParseDump(in, ioutil.Discard, true)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.E("stack.Aggregate", errors.K.NotExist, "reason", "no stacktrace found")
	}

	// stack.Augment prints stupid messages to std log...
	// replace stdlog writer with discard writer and reset immediately
	// afterwards
	w := stdlog.Writer()
	stdlog.SetOutput(ioutil.Discard)
	stack.Augment(c.Goroutines)
	stdlog.SetOutput(w)

	buckets := stack.Aggregate(c.Goroutines, sim)

	return &AggregateStack{
		trace:      trace,
		buckets:    buckets,
		similarity: sim,
	}, nil
}

func (a *AggregateStack) Buckets() []*stack.Bucket {
	return a.buckets
}

func (a *AggregateStack) String() string {
	s, _ := a.AsText()
	return s
}

func (a *AggregateStack) SimilarityString() string {
	switch a.similarity {
	case stack.ExactFlags:
		return "identical"
	case stack.ExactLines:
		return "exact"
	case stack.AnyPointer:
		return "normal"
	case stack.AnyValue:
		return "aggressive"
	}
	return "unknown"
}

// AsText converts this stack to a text based form.
func (a *AggregateStack) AsText() (string, error) {
	out := &bytes.Buffer{}
	err := a.writeAsText(out)
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// AsText converts this stack to an HTML page.
func (a *AggregateStack) AsHTML() (string, error) {
	out := &bytes.Buffer{}
	err := a.writeAsHTML(out)
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// SortByCount sorts the stack traces by the number of goroutines that were
// aggregated into the same stack trace.
func (a *AggregateStack) SortByCount(ascending bool) {
	sort.SliceStable(a.buckets, func(i, j int) bool {
		return numberutil.LessInt(ascending, len(a.buckets[i].IDs), len(a.buckets[j].IDs), func() bool {
			return numberutil.LessInt(ascending, a.buckets[i].SleepMax, a.buckets[j].SleepMax, func() bool {
				return numberutil.LessInt(ascending, a.buckets[i].SleepMin, a.buckets[j].SleepMin)
			})
		})
	})
}

// SortBySleepTime sorts the stack traces by their sleep times.
func (a *AggregateStack) SortBySleepTime(ascending bool) {
	sort.SliceStable(a.buckets, func(i, j int) bool {
		return numberutil.LessInt(ascending, a.buckets[i].SleepMax, a.buckets[j].SleepMax, func() bool {
			return numberutil.LessInt(ascending, a.buckets[i].SleepMin, a.buckets[j].SleepMin, func() bool {
				return numberutil.LessInt(ascending, len(a.buckets[i].IDs), len(a.buckets[j].IDs))
			})
		})
	})
}
