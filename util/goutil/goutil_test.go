package goutil

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apexlog "github.com/eluv-io/apexlog-go"
	"github.com/eluv-io/apexlog-go/handlers/memory"
	"github.com/eluv-io/common-go/util/timeutil"
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
	t.Run("single call", func(t *testing.T) {
		// single call, ensure entry/exit logging
		entries := doTestGo(1, "debug")
		require.Equal(t, 3, len(entries))

		require.Equal(t, "goroutine.enter process", entries[0].Message)
		fields := entries[0].Fields.Map()
		require.Equal(t, "context", fields["some"])
		require.Equal(t, "more", fields["and"])
		require.NotEmpty(t, fields["gid"])
		require.NotEmpty(t, fields["parent_gid"])

		require.Equal(t, "process", entries[1].Message)

		require.Equal(t, "goroutine.exit process", entries[2].Message)
		fields = entries[2].Fields.Map()
		require.Equal(t, "context", fields["some"])
		require.Equal(t, "more", fields["and"])
		require.NotEmpty(t, fields["gid"])
		require.NotEmpty(t, fields["parent_gid"])
	})
	t.Run("parallel calls", func(t *testing.T) {
		// parallel calls
		watch := timeutil.StartWatch()
		entries := doTestGo(10, "debug")
		require.Less(t, watch.Duration(), 2*time.Second)
		require.Equal(t, 30, len(entries))
	})
	t.Run("parallel calls, info", func(t *testing.T) {
		// parallel calls, no entry/exit logging due to log level
		watch := timeutil.StartWatch()
		entries := doTestGo(10, "info")
		require.Less(t, watch.Duration(), 2*time.Second)
		require.Equal(t, 10, len(entries))
	})
}

func doTestGo(count int, level string) []*apexlog.Entry {
	currentLog := log
	defer func() {
		log = currentLog
	}()

	goRoutineID := true
	log = elog.New(
		&elog.Config{
			Handler:     "memory",
			Level:       level,
			GoRoutineID: &goRoutineID,
		})

	wg := sync.WaitGroup{}
	for i := 0; i < count; i++ {
		wg.Add(1)
		Go("process", []any{"some", "context", "and", "more"},
			func() {
				log.Info("process")
				time.Sleep(time.Second)
				wg.Done()
			})
	}
	wg.Wait()

	// wait group is not sufficient because Go() logs after the passed in function returns...
	time.Sleep(20 * time.Millisecond)

	h := log.Handler().(*memory.Handler)
	return h.Entries
}
