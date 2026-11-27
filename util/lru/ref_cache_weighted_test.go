package lru

import (
	"fmt"
	"io"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/structured"
)

type weighedResource struct {
	key         string
	weight      atomic.Uint64
	constructed atomic.Int64
	closed      atomic.Int64
}

func newWeighedResource(key string, weight uint64) *weighedResource {
	r := &weighedResource{key: key}
	r.weight.Store(weight)
	return r
}

func (r *weighedResource) constructor() (*weighedResource, error) {
	r.constructed.Add(1)
	return r, nil
}

func (r *weighedResource) Close() error {
	r.closed.Add(1)
	return nil
}

func weigh(r *weighedResource) uint64 { return r.weight.Load() }

// newTestWeightedCache creates a weighted cache that runs otter's maintenance right away, so that evictions are
// deterministic.
func newTestWeightedCache(t *testing.T, maxWeight uint64) *TypedRefCache[string, *weighedResource] {
	c, err := newWeightedRefCache[string, *weighedResource](maxWeight, weigh, nil, func(fn func()) { fn() })
	require.NoError(t, err)
	return c
}

func get(t *testing.T, c *TypedRefCache[string, *weighedResource], r *weighedResource) {
	got, err := c.GetOrCreate(r.key, r.constructor)
	require.NoError(t, err)
	require.Same(t, r, got)
}

// settle runs otter's pending maintenance and closes the evicted entries.
func (c *TypedRefCache[K, V]) settle() {
	c.weighted.CleanUp()
	c.mutex.Lock()
	c.closeEvicted()
	c.mutex.Unlock()
}

func TestNewWeightedRefCacheInvalid(t *testing.T) {
	_, err := NewWeightedRefCache[string, *weighedResource](0, weigh, nil)
	require.Error(t, err)
	_, err = NewWeightedRefCache[string, *weighedResource](100, nil, nil)
	require.Error(t, err)
}

func TestWeightedRefCacheHits(t *testing.T) {
	c := newTestWeightedCache(t, 100)
	rs := make([]*weighedResource, 5)
	for i := range rs {
		rs[i] = newWeighedResource(fmt.Sprint("r", i), 10)
		get(t, c, rs[i])
		c.Release(rs[i].key)
	}
	for _, r := range rs {
		get(t, c, r)
		c.Release(r.key)
	}
	c.settle()
	for _, r := range rs {
		require.EqualValues(t, 1, r.constructed.Load())
		require.EqualValues(t, 0, r.closed.Load())
	}

	m := c.Metrics()
	require.EqualValues(t, 5, m.Hits.Load())
	require.EqualValues(t, 5, m.Misses.Load())
	require.EqualValues(t, 50, m.Weight.Load())

	generic := structured.Wrap(c.CollectMetrics().MarshalGeneric())
	require.Equal(t, 100, generic.At("config/max_weight").Int())
	require.Equal(t, 50, generic.At("weight").Int())
}

func TestWeightedRefCacheEvictsByWeight(t *testing.T) {
	c := newTestWeightedCache(t, 100)
	rs := make([]*weighedResource, 30)
	for i := range rs {
		rs[i] = newWeighedResource(fmt.Sprint("r", i), 10)
		get(t, c, rs[i])
		c.Release(rs[i].key)
	}
	c.settle()

	m := c.Metrics()
	require.LessOrEqual(t, m.Weight.Load(), int64(100))
	closed := 0
	for _, r := range rs {
		require.LessOrEqual(t, r.closed.Load(), int64(1))
		closed += int(r.closed.Load())
	}
	require.Equal(t, len(rs)-int(m.Weight.Load()/10), closed)
	require.EqualValues(t, closed, m.Removed.Load())

	c.Purge()
	for _, r := range rs {
		require.EqualValues(t, 1, r.closed.Load())
	}
}

func TestWeightedRefCacheOversize(t *testing.T) {
	c := newTestWeightedCache(t, 100)
	r := newWeighedResource("big", 101)
	get(t, c, r)
	require.EqualValues(t, 0, r.closed.Load())
	c.Release(r.key)
	c.settle()
	require.EqualValues(t, 1, r.closed.Load())
}

func TestWeightedRefCacheActiveNotClosed(t *testing.T) {
	c := newTestWeightedCache(t, 100)
	a := newWeighedResource("a", 50)
	get(t, c, a)
	c.Release(a.key)
	// a is active again and stays in the weighted cache while active
	get(t, c, a)

	// enough weight to evict a, used often enough to be admitted
	others := make([]*weighedResource, 20)
	for i := range others {
		others[i] = newWeighedResource(fmt.Sprint("o", i), 30)
		for j := 0; j < 5; j++ {
			get(t, c, others[i])
			c.Release(others[i].key)
		}
	}
	c.settle()
	require.EqualValues(t, 0, a.closed.Load(), "active resource closed")

	c.Release(a.key)
	c.settle()
	c.Purge()
	require.EqualValues(t, 1, a.closed.Load())
	require.EqualValues(t, 1, a.constructed.Load())
	for _, r := range others {
		require.EqualValues(t, r.constructed.Load(), r.closed.Load())
	}
}

func TestWeightedRefCacheReweigh(t *testing.T) {
	c := newTestWeightedCache(t, 100)
	r := newWeighedResource("r", 10)
	get(t, c, r)
	c.Release(r.key)
	c.settle()
	require.EqualValues(t, 10, c.Metrics().Weight.Load())

	r.weight.Store(40)
	c.Reweigh(r.key)
	c.settle()
	require.EqualValues(t, 40, c.Metrics().Weight.Load())
	require.EqualValues(t, 0, r.closed.Load())

	r.weight.Store(200)
	c.Reweigh(r.key)
	c.settle()
	require.EqualValues(t, 1, r.closed.Load())
	require.EqualValues(t, 0, c.Metrics().Weight.Load())

	// unknown keys are ignored
	c.Reweigh("unknown")
}

// TestWeightedRefCacheScanResistance verifies that scans over many resources used once don't evict the resources that
// are used frequently. Between two uses of the hot resources, the scan uses more resources than fit into the cache, so
// a plain LRU loses all hot resources.
func TestWeightedRefCacheScanResistance(t *testing.T) {
	keptHot := func(c *TypedRefCache[string, *weighedResource], settle func()) int {
		use := func(r *weighedResource) {
			get(t, c, r)
			c.Release(r.key)
		}
		// fill the cache: otter counts frequencies only once it is about half full
		for i := 0; i < 200; i++ {
			use(newWeighedResource(fmt.Sprint("fill", i), 1))
		}
		hot := make([]*weighedResource, 10)
		for i := range hot {
			hot[i] = newWeighedResource(fmt.Sprint("hot", i), 1)
		}
		for round := 0; round < 20; round++ {
			for _, r := range hot {
				use(r)
			}
			settle()
			for i := 0; i < 150; i++ {
				use(newWeighedResource(fmt.Sprint("scan", round, "-", i), 1))
			}
		}
		settle()

		// a hot resource is still cached if using it again doesn't construct it again
		kept := 0
		for _, r := range hot {
			constructed := r.constructed.Load()
			use(r)
			if r.constructed.Load() == constructed {
				kept++
			}
		}
		return kept
	}

	weighted := newTestWeightedCache(t, 100)
	require.GreaterOrEqual(t, keptHot(weighted, weighted.settle), 9, "hot resources evicted by the scan")

	plain := NewTypedRefCache[string, *weighedResource](100, nil)
	require.Equal(t, 0, keptHot(plain, func() {}))
}

func TestWeightedRefCacheConcurrent(t *testing.T) {
	c, err := NewWeightedRefCache[string, *weighedResource](200, weigh, nil)
	require.NoError(t, err)

	const keys = 100
	var mutex sync.Mutex
	created := map[*weighedResource]bool{}
	wg := sync.WaitGroup{}
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rnd := rand.New(rand.NewSource(seed))
			for i := 0; i < 2000; i++ {
				key := fmt.Sprint("k", rnd.Intn(keys))
				res, err := c.GetOrCreate(key, func() (*weighedResource, error) {
					r := newWeighedResource(key, uint64(1+rnd.Intn(20)))
					mutex.Lock()
					created[r] = true
					mutex.Unlock()
					return r, nil
				})
				require.NoError(t, err)
				require.EqualValues(t, 0, res.closed.Load(), "closed resource returned")
				if rnd.Intn(4) == 0 {
					res.weight.Store(uint64(1 + rnd.Intn(40)))
					c.Reweigh(key)
				}
				c.Release(key)
			}
		}(int64(g))
	}
	wg.Wait()

	c.weighted.CleanUp()
	c.Purge()
	m := c.Metrics()
	require.EqualValues(t, len(created), m.Added.Load())
	require.EqualValues(t, len(created), m.Removed.Load())
	for r := range created {
		require.EqualValues(t, 1, r.closed.Load(), "resource %s", r.key)
	}
}

// BenchmarkRefCacheHit measures a hit on an idle resource and its release, in a plain and in a weighted cache.
func BenchmarkRefCacheHit(b *testing.B) {
	weighted, err := NewWeightedRefCache[string, *weighedResource](1000, weigh, nil)
	require.NoError(b, err)
	caches := map[string]*TypedRefCache[string, *weighedResource]{
		"plain":    NewTypedRefCache[string, *weighedResource](1000, nil),
		"weighted": weighted,
	}
	for name, c := range caches {
		b.Run(name, func(b *testing.B) {
			keys := make([]string, 100)
			for i := range keys {
				r := newWeighedResource(fmt.Sprint("r", i), 1)
				keys[i] = r.key
				_, err := c.GetOrCreate(r.key, r.constructor)
				require.NoError(b, err)
				c.Release(r.key)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				key := keys[i%len(keys)]
				_, err := c.GetOrCreate(key, nil)
				if err != nil {
					b.Fatal(err)
				}
				c.Release(key)
			}
		})
	}
}

var _ io.Closer = (*weighedResource)(nil)
