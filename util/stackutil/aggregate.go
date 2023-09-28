package stackutil

import (
	"bytes"
	"io"
	"sort"

	"github.com/maruel/panicparse/v2/stack"

	"github.com/eluv-io/errors-go"

	"github.com/eluv-io/common-go/util/numberutil"
)

const (
	Normal     = stack.AnyPointer
	Aggressive = stack.AnyValue
)

// AggregateStack is an aggregated stack dump - a wrapper around the "buckets" of the 3rd-party panicparse library.
type AggregateStack struct {
	trace      string
	agg        *stack.Aggregated
	similarity stack.Similarity
}

// Aggregate creates an aggregated stack dump object from the given stack trace. The aggregation will be performed more
// or less aggressively based on the provided similarity parameter.
func Aggregate(trace string, sim stack.Similarity) (*AggregateStack, error) {
	in := bytes.NewBuffer([]byte(trace))

	snapshot, _, err := stack.ScanSnapshot(in, io.Discard, stack.DefaultOpts())
	if snapshot == nil {
		return nil, errors.E("stack.Aggregate", errors.K.NotExist, err, "reason", "no stacktrace found")
	}

	agg := snapshot.Aggregate(sim)

	return &AggregateStack{
		trace:      trace,
		agg:        agg,
		similarity: sim,
	}, nil
}

func (a *AggregateStack) Buckets() []*stack.Bucket {
	return a.agg.Buckets
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

// AsHTML converts this stack to an HTML page.
func (a *AggregateStack) AsHTML() (string, error) {
	out := &bytes.Buffer{}
	err := a.agg.ToHTML(out, "")
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// SortByCount sorts the stack traces by the number of goroutines that were
// aggregated into the same stack trace.
func (a *AggregateStack) SortByCount(ascending bool) {
	buckets := a.agg.Buckets
	sort.SliceStable(buckets, func(i, j int) bool {
		return numberutil.LessInt(ascending, len(buckets[i].IDs), len(buckets[j].IDs), func() bool {
			return numberutil.LessInt(ascending, buckets[i].SleepMax, buckets[j].SleepMax, func() bool {
				return numberutil.LessInt(ascending, buckets[i].SleepMin, buckets[j].SleepMin)
			})
		})
	})
}

// SortBySleepTime sorts the stack traces by their sleep times.
func (a *AggregateStack) SortBySleepTime(ascending bool) {
	buckets := a.agg.Buckets
	sort.SliceStable(buckets, func(i, j int) bool {
		return numberutil.LessInt(ascending, buckets[i].SleepMax, buckets[j].SleepMax, func() bool {
			return numberutil.LessInt(ascending, buckets[i].SleepMin, buckets[j].SleepMin, func() bool {
				return numberutil.LessInt(ascending, len(buckets[i].IDs), len(buckets[j].IDs))
			})
		})
	})
}
