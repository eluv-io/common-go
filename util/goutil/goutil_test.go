package goutil

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/apexlog-go/handlers/memory"
	elog "github.com/eluv-io/log-go"
)

// BenchmarkGoID
// BenchmarkGoID-8   	803637735	         1.375 ns/op	       0 B/op	       0 allocs/op
func BenchmarkGoID(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GoID()
	}
}

// TestGoID just calls GoID.
// The underlying library panics when it cannot load.
func TestGoID(t *testing.T) {
	id1 := GoID()
	id2 := id1

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		id2 = GoID()
		wg.Done()
	}()
	wg.Wait()

	require.NotEqual(t, id1, id2)
}

func TestGo(t *testing.T) {
	currentLog := log
	defer func() {
		log = currentLog
	}()

	goRoutineID := true
	log = elog.New(
		&elog.Config{
			Handler:     "memory",
			Level:       "debug",
			GoRoutineID: &goRoutineID,
		})

	wg := sync.WaitGroup{}
	wg.Add(1)
	Go("process", []any{"some", "context", "and", "more"},
		func() {
			log.Info("process")
			wg.Done()
		})
	wg.Wait()

	h := log.Handler().(*memory.Handler)
	require.Len(t, h.Entries, 3)

	require.Equal(t, "goroutine.enter process", h.Entries[0].Message)
	fields := h.Entries[0].Fields.Map()
	require.Equal(t, "context", fields["some"])
	require.Equal(t, "more", fields["and"])
	require.NotEmpty(t, fields["gid"])
	require.NotEmpty(t, fields["parent_gid"])

	require.Equal(t, "process", h.Entries[1].Message)

	require.Equal(t, "goroutine.exit process", h.Entries[2].Message)
	fields = h.Entries[2].Fields.Map()
	require.Equal(t, "context", fields["some"])
	require.Equal(t, "more", fields["and"])
	require.NotEmpty(t, fields["gid"])
	require.NotEmpty(t, fields["parent_gid"])
}
