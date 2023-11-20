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

// NewSnapshot creates a snapshot by parsing stacktrace information from the given string. Returns an error if no
// stacktraces are found.
func NewSnapshot(trace string) (*Snapshot, error) {
	in := bytes.NewBuffer([]byte(trace))
	snapshot, _, err := stack.ScanSnapshot(in, io.Discard, stack.DefaultOpts())
	if snapshot == nil {
		return nil, errors.E("stack.CreateSnapshot", errors.K.NotExist, "error", err, "reason", "no stacktrace found")
	}
	s := &Snapshot{snapshot, utc.Now()}
	s.SortByGID(true)
	return s, nil
}

// ExtractSnapshots extracts all snapshot found in the given reader. In the case of an error, the snapshots found up to
// that point are returned.
func ExtractSnapshots(reader io.Reader) ([]*Snapshot, error) {
	res := make([]*Snapshot, 0, 10)
	for {
		snapshot, _, err := stack.ScanSnapshot(reader, io.Discard, stack.DefaultOpts())
		if snapshot == nil {
			if err != io.EOF {
				return res, errors.E("stack.ExtractSnapshots", errors.K.IO, err)
			}
			break
		}
		s := &Snapshot{snapshot, utc.Now()}
		s.SortByGID(true)
		res = append(res, s)
	}
	return res, nil
}

// Snapshot is a wrapper around github.com/maruel/panicparse/v2/stack.Snapshot and offers sorting, filtering
// and custom text marshalling.
type Snapshot struct {
	*stack.Snapshot
	Timestamp utc.UTC
}

// SortByGID sorts the goroutines by goroutine ID.
func (s *Snapshot) SortByGID(ascending bool) {
	goroutines := s.Goroutines
	sort.SliceStable(goroutines, func(i, j int) bool {
		return numberutil.LessInt(ascending, goroutines[i].ID, goroutines[j].ID)
	})
}

// AsText converts this stack to a text based form.
func (s *Snapshot) String() string {
	res, _ := s.AsText()
	return res
}

// AsText converts this stack to a text based form.
func (s *Snapshot) AsText() (string, error) {
	out := &bytes.Buffer{}
	err := s.writeAsText(out)
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// Filter filters the Snapshot by retaining the Goroutines that match the given "keep" function and removing all
// others. It returns the number of removed Goroutines.
func (s *Snapshot) Filter(keep func(goroutine *stack.Goroutine) bool) (removed int) {
	remove := func(e *stack.Goroutine) bool {
		return !keep(e)
	}
	s.Goroutines, removed = sliceutil.RemoveMatch(s.Goroutines, remove)
	return removed
}

// FilterText filters the Snapshot by retaining (keep=true) or removing (keep=false) the Goroutines that match the given "match"
// strings. A Goroutine matches if at least one of its function calls or corresponding source
// filenames contains at least one of the "match" strings. The return value is the number of removed Goroutines.
func (s *Snapshot) FilterText(keep bool, match ...string) (removed int) {
	s.Goroutines, removed = sliceutil.RemoveMatch(s.Goroutines, func(e *stack.Goroutine) bool {
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

// Aggregate creates an aggregated stack dump from this snapshot. The aggregation is more or less aggressive based on
// the provided similarity parameter.
func (s *Snapshot) Aggregate(sim stack.Similarity) *AggregateStack {
	agg := s.Snapshot.Aggregate(sim)
	return &AggregateStack{
		Agg:        agg,
		Similarity: sim,
		Timestamp:  s.Timestamp,
	}
}
