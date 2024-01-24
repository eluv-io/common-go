package lru_test

import (
	"fmt"
	"io"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/eluv-io/common-go/util/syncutil"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/format/structured"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/lru"
)

func TestNewRefCacheBasic(t *testing.T) {
	cb := &callback{}
	cache := lru.NewRefCache(2, cb)

	r1 := newResource("r1")
	r2 := newResource("r2")
	r3 := newResource("r3")
	r4 := newResource("r4")

	assertGet := func(res *resource) {
		got, err := cache.GetOrCreate(res.key, res.constructor)
		require.NoError(t, err)
		require.Same(t, res, got)
	}

	assertGet(r1)
	assertGet(r1)
	assertGet(r1)
	cache.Release(r1.key) // partial release while in LRU

	assertGet(r2)

	assertGet(r3)
	cache.Release(r3.key) // final release while in LRU

	assertGet(r4)

	cache.Release(r1.key) // partial release while evicted from LRU
	cache.Release(r1.key) // final release while evicted from LRU

	cache.Release(r2.key) // final release while evicted from LRU
	cache.Release(r4.key) // final release while evicted from LRU

	cache.Purge()

	require.Equal(t, cb.open.Load(), cb.close.Load())
	require.Equal(t, cb.get.Load(), cb.release.Load())

	r1.assert(t, 1, 1)
	r2.assert(t, 1, 1)
	r3.assert(t, 1, 1)
	r4.assert(t, 1, 1)

	{
		metrics := cache.Metrics()
		require.EqualValues(t, 2, metrics.Hits.Load())
		require.EqualValues(t, 4, metrics.Misses.Load())
		require.EqualValues(t, 0, metrics.Errors.Load())
		require.EqualValues(t, 4, metrics.Added.Load())
		require.EqualValues(t, 4, metrics.Removed.Load())
	}

	{
		metrics := structured.Wrap(cache.CollectMetrics().MarshalGeneric())
		require.Equal(t, 2, metrics.At("hits").Int())
		require.Equal(t, 4, metrics.At("misses").Int())
		require.Equal(t, 0, metrics.At("errors").Int())
		require.Equal(t, 4, metrics.At("added").Int())
		require.Equal(t, 4, metrics.At("removed").Int())
	}
}

func TestConcurrent(t *testing.T) {
	// log.Get("/").SetDebug()

	const (
		workers   = 2
		cacheSize = 10
		keyRange  = 20
		runTime   = "2s"
		loops     = 5000
		// constructionDelay = "5ms"
	)
	runDuration := duration.MustParse(runTime).Duration()
	// constDelay := duration.MustParse(constructionDelay).Duration()

	cb := &callback{}
	cache := lru.NewRefCache(cacheSize, cb)

	resources := make([]*resource, keyRange)
	for i := 0; i < keyRange; i++ {
		key := fmt.Sprintf("k%.2d", i)
		resources[i] = newResource(key)
	}

	stopChan := make(chan bool)
	resChan := make(chan [2]int)
	for j := 0; j < workers; j++ {
		go func() {
			var count, evictedCount int
			for {
				select {
				case <-stopChan:
					resChan <- [2]int{count, evictedCount}
					return
				default:
					break
				}
				idx := rand.Intn(keyRange)
				// idx := i % keyRange
				res := resources[idx]
				val, err := cache.GetOrCreate(res.key, res.constructor)
				//require.NoError(t, err)
				//require.Same(t, res, val)
				_ = val
				_ = err
				count++
				evicted := cache.Release(res.key)
				if evicted {
					evictedCount++
				}
			}
		}()
	}
	time.Sleep(runDuration)
	close(stopChan)

	var totalCount, evictedCount int
	for j := 0; j < workers; j++ {
		res := <-resChan
		totalCount += res[0]
		evictedCount += res[1]
	}

	fmt.Println("total", totalCount, "evicted", evictedCount)
	fmt.Println("opened", cb.open.Load(),
		"closed", cb.close.Load(),
		"got", cb.get.Load(),
		"release", cb.release.Load())
	metrics := cache.Metrics()
	fmt.Println(jsonutil.MarshalString(metrics))
	require.EqualValues(t, totalCount, metrics.Hits.Load()+metrics.Misses.Load())
	require.EqualValues(t, 0, metrics.Errors.Load())
	require.EqualValues(t, cacheSize, metrics.Config.MaxItems)
	require.EqualValues(t, cacheSize, metrics.Added.Load()-metrics.Removed.Load())

	cache.Purge()

	metrics = cache.Metrics()
	require.EqualValues(t, 0, metrics.Added.Load()-metrics.Removed.Load())
	require.Equal(t, cb.open.Load(), cb.close.Load())
	require.Equal(t, cb.get.Load(), cb.release.Load())
}

func TestConstructionConcurrency(t *testing.T) {
	cb := &callback{}
	cache := lru.NewRefCache(2, cb)

	wg := &sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		i := i
		wg.Add(1)
		go func() {
			key := fmt.Sprintf("key-%.2d", i)
			res, err := cache.GetOrCreate(key, func() (io.Closer, error) {
				time.Sleep(time.Second)
				return newResource(key), nil
			})
			require.NoError(t, err)
			require.Equal(t, key, res.(*resource).key)
			wg.Done()
		}()
	}
	require.False(t, syncutil.WaitTimeout(wg, 2*time.Second))

}
func newResource(key string) *resource {
	return &resource{key: key}
}

type resource struct {
	key         string
	constructed atomic.Int64
	closed      atomic.Int64
}

func (r *resource) constructor() (io.Closer, error) {
	r.constructed.Inc()
	return r, nil
}

func (r *resource) Close() error {
	r.closed.Inc()
	return nil
}

func (r *resource) assert(t testing.TB, constructed, closed int) {
	require.EqualValues(t, constructed, r.constructed.Load())
	require.EqualValues(t, closed, r.closed.Load())
}

type callback struct {
	open    atomic.Int64
	close   atomic.Int64
	get     atomic.Int64
	release atomic.Int64
}

func (c *callback) OnOpen() {
	c.open.Inc()
}

func (c *callback) OnGet() {
	c.get.Inc()
}

func (c *callback) OnRelease() {
	c.release.Inc()
}

func (c *callback) OnClose() {
	c.close.Inc()
}
