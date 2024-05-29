package stackutil

import (
	"bytes"
	"io"
	"sort"
	"strings"

	"github.com/maruel/panicparse/v2/stack"

	"github.com/eluv-io/common-go/util/numberutil"
	"github.com/eluv-io/common-go/util/sliceutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
)

const (
	Normal     = stack.AnyPointer
	Aggressive = stack.AnyValue
)

// AggregateStack is an aggregated stack dump - a wrapper around the "buckets" of the 3rd-party panicparse library.
type AggregateStack struct {
	Agg        *stack.Aggregated
	Similarity stack.Similarity
	Timestamp  utc.UTC
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
		Agg:        agg,
		Similarity: sim,
		Timestamp:  utc.Now(),
	}, nil
}

func (a *AggregateStack) Buckets() []*stack.Bucket {
	return a.Agg.Buckets
}

func (a *AggregateStack) String() string {
	s, _ := a.AsText()
	return s
}

func (a *AggregateStack) SimilarityString() string {
	switch a.Similarity {
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
	err := a.Agg.ToHTML(out, "")
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// SortByCount sorts the stack traces by the number of goroutines that were
// aggregated into the same stack trace.
func (a *AggregateStack) SortByCount(ascending bool) {
	buckets := a.Agg.Buckets
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
	buckets := a.Agg.Buckets
	sort.SliceStable(buckets, func(i, j int) bool {
		return numberutil.LessInt(ascending, buckets[i].SleepMax, buckets[j].SleepMax, func() bool {
			return numberutil.LessInt(ascending, buckets[i].SleepMin, buckets[j].SleepMin, func() bool {
				return numberutil.LessInt(ascending, len(buckets[i].IDs), len(buckets[j].IDs))
			})
		})
	})
}

// Filter filters the AggregateStack by retaining the buckets that match the given "keep" function and removing all
// others. It returns the number of removed buckets.
func (a *AggregateStack) Filter(keep func(*stack.Bucket) bool) (removed int) {
	remove := func(e *stack.Bucket) bool {
		return !keep(e)
	}
	a.Agg.Buckets, removed = sliceutil.RemoveMatch(a.Agg.Buckets, remove)
	return removed
}

// FilterText filters the AggregateStack by retaining (keep=true) or removing (keep=false) the buckets (aggregated call
// stacks) that match the given "match" strings. A bucket matches if at least one of its function calls or corresponding
// source filenames contains at least one of the "match" strings. The return value is the number of removed buckets.
func (a *AggregateStack) FilterText(keep bool, match ...string) (removed int) {
	a.Agg.Buckets, removed = sliceutil.RemoveMatch(a.Agg.Buckets, func(e *stack.Bucket) bool {
		for _, call := range e.Stack.Calls {
			for _, s := range match {
				if strings.Contains(call.SrcName, s) || strings.Contains(call.Func.Complete, s) {
					return !keep
				}
			}
		}
		return keep
	})
	return removed
}
