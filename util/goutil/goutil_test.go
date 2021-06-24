package goutil

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

//BenchmarkGoID
//BenchmarkGoID-8   	803637735	         1.375 ns/op	       0 B/op	       0 allocs/op
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
