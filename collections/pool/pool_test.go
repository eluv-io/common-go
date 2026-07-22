package pool

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPool(t *testing.T) {
	pool := New(&DatagramFactory{})
	res := pool.Borrow()
	require.EqualValues(t, 1, res.refs.Load())
	require.Len(t, res.T.Data, 1024)

	res.T.Data = res.T.Data[10:20]
	res.T.Data[0] = 1
	res.T.Data[9] = 10

	res.Reference()
	require.EqualValues(t, 2, res.refs.Load())
	res.Release()
	require.EqualValues(t, 1, res.refs.Load())
	res.ReferenceN(2)
	require.EqualValues(t, 3, res.refs.Load())
	res.ReleaseN(3)

	require.Panics(t, func() { res.Release() })

	res2 := pool.Borrow()
	require.EqualValues(t, 1, res2.refs.Load())
	require.Len(t, res2.T.Data, 1024)
}

// TestConcurrentReferenceRelease exercises the atomic refcounting under -race: the owner shares one resource with many
// workers, each of which releases its reference. The resource must be returned to the pool exactly once.
func TestConcurrentReferenceRelease(t *testing.T) {
	pool := New(&DatagramFactory{})
	res := pool.Borrow()

	const workers = 64
	res.ReferenceN(workers) // one reference per worker, on top of the owner's reference

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res.Release()
		}()
	}
	wg.Wait()

	require.EqualValues(t, 1, res.refs.Load()) // only the owner's reference remains
	require.EqualValues(t, 0, pool.Stats().Returned)

	res.Release() // owner releases -> returned to the pool
	require.EqualValues(t, 1, pool.Stats().Returned)
}

func TestReferenceReleaseNegativeCount(t *testing.T) {
	pool := New(&DatagramFactory{})
	res := pool.Borrow()
	defer res.Release()

	// A negative count would silently corrupt the ref count (and could spuriously return the resource to the pool), so
	// it is rejected as a programming error.
	require.Panics(t, func() { res.ReferenceN(-1) })
	require.Panics(t, func() { res.ReleaseN(-1) })
	require.EqualValues(t, 1, res.refs.Load()) // ref count unchanged
}

func TestStats(t *testing.T) {
	pool := New(&DatagramFactory{})
	require.Equal(t, Stats{}, pool.Stats())

	// Borrowed and Returned are deterministic (incremented on every Borrow / final Release). Created counts pool
	// misses (factory.New calls), which is NOT deterministic: sync.Pool may drop pooled resources at any GC, so a later
	// Borrow may miss and allocate again. We therefore assert Created only within bounds: at least 1, at most Borrowed.

	// first borrow: a pool miss -> created and borrowed both increment
	r1 := pool.Borrow()
	require.Equal(t, int64(1), pool.Stats().Created) // first borrow on an empty pool always misses
	require.Equal(t, int64(1), pool.Stats().Borrowed)
	require.Equal(t, int64(0), pool.Stats().Returned)

	// share and release: only the final release returns the resource to the pool
	r1.Reference()
	r1.Release()
	require.Equal(t, int64(0), pool.Stats().Returned)
	r1.Release()
	require.Equal(t, int64(1), pool.Stats().Returned)

	// second borrow: borrowed increments; created increments only if the returned resource was not reused
	r2 := pool.Borrow()
	require.Equal(t, int64(2), pool.Stats().Borrowed)
	r2.Release()
	require.Equal(t, int64(2), pool.Stats().Returned)

	final := pool.Stats()
	require.GreaterOrEqual(t, final.Created, int64(1))
	require.LessOrEqual(t, final.Created, final.Borrowed)
}

type Datagram struct {
	Data []byte
	data []byte
}

type DatagramFactory struct{}

func (f *DatagramFactory) New() *Datagram {
	bts := make([]byte, 1024)
	return &Datagram{
		data: bts,
	}
}

func (f *DatagramFactory) Init(t *Datagram) {
	t.Data = t.data
}

func (f *DatagramFactory) Reset(t *Datagram) {
	t.Data = nil
}
