package lru_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/lru"
	"github.com/eluv-io/common-go/util/timeutil"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

var log = func() *elog.Log {
	c := elog.NewConfig()
	c.Handler = "text"
	return elog.New(c)
}()

func TestExpiringCache(t *testing.T) {
	testNilCache(t, lru.NewExpiringCache(0, 5))
	testNilCache(t, lru.NewExpiringCache(5, 0))
	testNilCache(t, lru.NewExpiringCache(0, 0))
	testExpiringCache(t, nil)
}

func testNilCache(t *testing.T, cache *lru.ExpiringCache) {
	key := "key"
	value := "value"

	for i := 0; i < 10; i++ {
		val, evicted, err := cache.GetOrCreate(key, func() (interface{}, error) {
			return value, nil
		})
		require.NoError(t, err)
		require.Equal(t, false, evicted)
		require.Equal(t, value, val)
	}

	require.False(t, cache.Add(key, value))
	require.Equal(t, 0, cache.Len())

	val, evicted := cache.Get(key)
	require.Nil(t, val)
	require.False(t, evicted)

	isNew, evicted := cache.Update(key, value)
	require.True(t, isNew)
	require.False(t, evicted)

	cache.Remove("another key")
	cache.Purge()
	require.Nil(t, cache.Entries())

	cache.EvictExpired()
	m := lru.MakeMetrics()
	require.Equal(t, m, cache.Metrics())
	require.Equal(t, &m, cache.CollectMetrics())
}

func TestExpiringCacheAssertEntries(t *testing.T) {
	testExpiringCache(t, func(cache *lru.ExpiringCache, valAndDates ...interface{}) {
		require.Equal(t, len(valAndDates)/2, cache.Len())
		entries := cache.Entries()
		require.Equal(t, len(valAndDates)/2, len(entries))
		for i := 0; i < len(valAndDates); i += 2 {
			require.Equal(t, valAndDates[i], entries[i/2].Value())
			require.Equal(t, valAndDates[i+1], entries[i/2].LastUpdated())
		}
	})
}

func testExpiringCache(t *testing.T, assertEntries func(c *lru.ExpiringCache, valAndDates ...interface{})) {
	if assertEntries == nil {
		assertEntries = func(c *lru.ExpiringCache, valAndDates ...interface{}) {} // no-op
	}

	defer utc.ResetNow()

	var evictedCount int

	now := utc.Now()
	t0 := now

	utc.MockNowFn(func() utc.UTC {
		return now
	})

	cache := lru.NewExpiringCache(10, duration.Spec(5*time.Second))
	cache.WithEvictHandler(func(key any, entry lru.ExpiringEntry[any]) {
		evictedCount++
	})

	cstr := func(v interface{}) func() (interface{}, error) {
		return func() (interface{}, error) {
			return v, nil
		}
	}
	assertGoC := func(key interface{}, constructor func() (interface{}, error), wantVal interface{}, wantEviction bool) {
		res, evicted, err := cache.GetOrCreate(key, constructor)
		require.NoError(t, err)
		require.Equal(t, wantVal, res)
		require.Equal(t, wantEviction, evicted)
	}

	assertEntries(cache)

	assertGoC("k1", cstr("v1"), "v1", false)
	assertEntries(cache, "v1", t0)

	now = now.Add(time.Second)
	t1 := now

	assertGoC("k1", nil, "v1", false)
	assertGoC("k1", nil, "v1", false)
	assertGoC("k2", cstr("v2"), "v2", false)
	assertGoC("k2", nil, "v2", false)

	assertEntries(cache, "v1", t0, "v2", t1)

	now = now.Add(4 * time.Second)
	t5 := now

	assertEntries(cache, "v2", t1)

	assertGoC("k1", cstr("v1.1"), "v1.1", false)
	assertGoC("k2", cstr("v2"), "v2", false)

	assertEntries(cache, "v1.1", t5, "v2", t1)

	now = now.Add(time.Second)
	t6 := now

	assertGoC("k2", cstr("v2.1"), "v2.1", false)
	assertEntries(cache, "v1.1", t5, "v2.1", t6)

	now = now.Add(5 * time.Second)
	assertEntries(cache)

	require.Equal(t, 0, cache.Len())
	require.Equal(t, 4, evictedCount)
}

func TestExpiringCacheResetOnAccess(t *testing.T) {
	defer utc.ResetNow()

	now := utc.Now()
	utc.MockNowFn(func() utc.UTC {
		return now
	})

	cache := lru.NewExpiringCache(10, duration.Spec(5*time.Second)).WithResetAgeOnAccess(true)

	cstr := func(v interface{}) func() (interface{}, error) {
		return func() (interface{}, error) {
			return v, nil
		}
	}
	assertGoC := func(key interface{}, constructor func() (interface{}, error), wantVal interface{}, wantEviction bool) {
		res, evicted, err := cache.GetOrCreate(key, constructor)
		require.NoError(t, err)
		require.Equal(t, wantVal, res)
		require.Equal(t, wantEviction, evicted)
	}
	assertGet := func(key interface{}, wantVal interface{}, wantExists bool) {
		res, ok := cache.Get(key)
		require.Equal(t, wantExists, ok)
		require.Equal(t, wantVal, res)
	}

	assertGoC("k1", cstr("v1"), "v1", false)

	now = now.Add(time.Second)

	assertGet("k1", "v1", true)
	assertGet("k1", "v1", true)
	assertGoC("k2", cstr("v2"), "v2", false)
	assertGet("k2", "v2", true)

	now = now.Add(4 * time.Second)

	// still not expired, because age was reset on last access...
	assertGet("k1", "v1", true)
	assertGet("k2", "v2", true)

	now = now.Add(5 * time.Second)

	assertGoC("k1", cstr("v1.1"), "v1.1", false)
	assertGoC("k2", cstr("v2.1"), "v2.1", false)

	now = now.Add(5 * time.Second)

	assertGet("k1", nil, false)
	assertGoC("k2", cstr("v2.2"), "v2.2", false)
	assertGet("k2", "v2.2", true)

	now = now.Add(5 * time.Second)
	assertGet("k1", nil, false)
	assertGet("k2", nil, false)

	cache.Add("k3", "v3")
	assertGet("k3", "v3", true)

	now = now.Add(5 * time.Second)
	assertGet("k3", nil, false)
}

// TestExpiringCacheResetAgeAfterCreation tests the expiring cache with the ResetAgeAfterCreation option enabled. It
// launches 2 groups of 3 clients (6 total). Each group accesses the same key. The cache expiration is set to 1 second,
// and the constructor function takes 1 second to complete. The test runs for 5.5 seconds, during which each client
// repeatedly accesses the cache every 10 milliseconds. The expected behavior is that each group of clients will trigger
// a refresh of their shared key every 2 seconds, and during the refresh, they will block.
// Each client should experience 3 long waits (the initial cache miss) and have a low average wait time overall.
func TestExpiringCacheResetAgeAfterCreation(t *testing.T) {
	cache := lru.NewTypedExpiringCache[string, int32](10, duration.Spec(1*time.Second)).
		WithMode(lru.Modes.Decoupled).
		WithResetAgeAfterCreation(true)

	var seq atomic.Int32

	clients := startAndRunClients(cache, &seq)

	for _, cl := range clients {
		cl.assert(t)
		avgWait := cl.avgWait()
		log.Info("stats",
			"client", cl.name,
			"invocations", cl.invocations,
			"long_waits", cl.longWaits,
			"avg_wait", avgWait,
			"total_wait", cl.waitTotal)
		require.InDelta(t, 12*time.Millisecond, avgWait, float64(2*time.Millisecond))
		require.InDelta(t, 3*time.Second, cl.waitTotal, float64(50*time.Millisecond))
		require.InDelta(t, 250, cl.invocations, float64(50)) // 5.5s - 1s initial wait
	}

	metrics := cache.Metrics()
	log.Info("metrics", "metrics", jsonutil.Stringer(&metrics))
	require.EqualValues(t, 6, metrics.Misses.Load())    // 6 misses total: 2 keys, each missing at 0s, 2s, and 4s
	require.InDelta(t, 1500, metrics.Hits.Load(), 50)   // seconds 2, 4, and half of 6 => 6 clients * 250 = 1500
	require.InDelta(t, 0, metrics.StaleHits.Load(), 30) // No stale hits expected in ResetAgeAfterCreation mode
}

// TestExpiringCacheServeStaleDuringRefresh tests the expiring cache with the ServeStaleDuringRefresh option enabled. It
// launches 2 groups of 3 clients (6 total). Each group accesses the same key. The cache expiration is set to 1 second,
// and the constructor function takes 1 second to complete. The test runs for 5.5 seconds, during which each client
// repeatedly accesses the cache every 10 milliseconds. The expected behavior is that each group of clients will trigger
// a refresh of their shared key every 2 seconds, and during the refresh, they will receive stale data without blocking.
// Each client should experience only one long wait (the initial cache miss) and have a low average wait time overall.
func TestExpiringCacheServeStaleDuringRefresh(t *testing.T) {
	t.Run("GetOrCreate()", func(t *testing.T) {
		var seq atomic.Int32
		cache := lru.NewTypedExpiringCache[string, int32](10, duration.Spec(1*time.Second)).
			WithMode(lru.Modes.Decoupled).
			WithResetAgeAfterCreation(true).
			WithServeStaleDuringRefresh(true)

		clients := startAndRunClients(cache, &seq)

		require.EqualValues(t, 6, seq.Load())

		for _, cl := range clients {
			cl.assert(t)
			avgWait := cl.avgWait()
			log.Info("stats",
				"client", cl.name,
				"key", cl.key,
				"last_val", cl.previousVal,
				"invocations", cl.invocations,
				"long_waits", cl.longWaits,
				"avg_wait", avgWait,
				"total_wait", cl.waitTotal)
			// require.Less(t, avgWait, 10*time.Millisecond)
			// require.Equal(t, 1, cl.longWaits)
			// require.InDelta(t, time.Second, cl.waitTotal, float64(20*time.Millisecond))
			// require.InDelta(t, 450, cl.invocations, float64(50)) // 5.5s - 1s initial wait
			// require.InDelta(t, 6, cl.previousVal, float64(1))    // shared sequence (so last val has to be 5 or 6
		}

		metrics := cache.Metrics()
		log.Info("metrics", "metrics", jsonutil.Stringer(&metrics))
		require.EqualValues(t, 2, metrics.Misses.Load())       // two misses at the beginning
		require.InDelta(t, 1500, metrics.Hits.Load(), 50)      // seconds 2, 4, half of 6 => 6 clients * 250 = 1500
		require.InDelta(t, 1200, metrics.StaleHits.Load(), 30) // seconds 3 & 5 => 6*200=1200
	})

	t.Run("Get()", func(t *testing.T) {
		var seq atomic.Int32
		cache := lru.NewTypedExpiringCache[string, int32](10, duration.Spec(1*time.Second)).
			WithMode(lru.Modes.Decoupled).
			WithResetAgeAfterCreation(true).
			WithServeStaleDuringRefresh(true)
		constructor := func() (int32, error) {
			log.Info("refreshing")
			time.Sleep(time.Second)
			res := seq.Add(1)
			log.Info("refreshed", "new", res)
			return res, nil
		}

		val, evicted, err := cache.GetOrCreate("a", constructor) // => miss
		require.NoError(t, err)
		require.False(t, evicted)
		require.EqualValues(t, 1, val)

		val, ok := cache.Get("a") // => hit
		require.True(t, ok)
		require.EqualValues(t, 1, val)

		time.Sleep(time.Second + 50*time.Millisecond)

		// entry is now expired: trigger refresh, receive stale value
		val, evicted, err = cache.GetOrCreate("a", constructor) // => stale
		require.NoError(t, err)
		require.False(t, evicted)
		require.EqualValues(t, 1, val)

		// Get also returns stale value
		val, ok = cache.Get("a") // => stale
		require.True(t, ok)
		require.EqualValues(t, 1, val)

		time.Sleep(time.Second + 50*time.Millisecond)

		// Get now returns fresh value
		val, ok = cache.Get("a") // => hit
		require.True(t, ok)
		require.EqualValues(t, 2, val)

		time.Sleep(time.Second + 50*time.Millisecond)

		// entry is now expired, no refresh going on ==> expect no value
		val, ok = cache.Get("a") // => miss
		require.False(t, ok)
		require.EqualValues(t, 0, val)

		m := cache.Metrics()
		log.Info("metrics", "metrics", jsonutil.Stringer(&m))
		require.EqualValues(t, 2, m.Hits.Load())
		require.EqualValues(t, 2, m.Misses.Load())
		require.EqualValues(t, 2, m.StaleHits.Load())
		require.EqualValues(t, 1, m.Added.Load())
		require.EqualValues(t, 1, m.Removed.Load())
		require.EqualValues(t, 0, cache.Len())
	})

	t.Run("evict refreshed but stale entry", func(t *testing.T) {
		var seq atomic.Int32
		cache := lru.NewTypedExpiringCache[string, int32](10, duration.Spec(1*time.Second)).
			WithMode(lru.Modes.Decoupled).
			WithResetAgeAfterCreation(true).
			WithServeStaleDuringRefresh(true)
		constructor := func() (int32, error) {
			log.Info("refreshing")
			time.Sleep(time.Second)
			res := seq.Add(1)
			log.Info("refreshed", "new", res)
			return res, nil
		}

		val, evicted, err := cache.GetOrCreate("b", constructor) // => miss
		require.NoError(t, err)
		require.False(t, evicted)
		require.EqualValues(t, 1, val)

		time.Sleep(time.Second + 50*time.Millisecond)

		// entry is now expired: trigger refresh, receive stale value
		val, evicted, err = cache.GetOrCreate("b", constructor) // => stale
		require.NoError(t, err)
		require.False(t, evicted)
		require.EqualValues(t, 1, val)

		// wait for refresh to complete and refreshed entry to expire
		time.Sleep(2*time.Second + 50*time.Millisecond)

		// expect new value
		val, evicted, err = cache.GetOrCreate("b", constructor) // => miss
		require.NoError(t, err)
		require.False(t, evicted)
		require.EqualValues(t, 3, val)

		m := cache.Metrics()
		log.Info("metrics", "metrics", jsonutil.Stringer(&m))
		require.EqualValues(t, 0, m.Hits.Load())
		require.EqualValues(t, 2, m.Misses.Load())
		require.EqualValues(t, 1, m.StaleHits.Load())
		require.EqualValues(t, 2, m.Added.Load())
		require.EqualValues(t, 1, m.Removed.Load())
		require.EqualValues(t, 1, cache.Len())
	})
}

func startAndRunClients(cache *lru.TypedExpiringCache[string, int32], seq *atomic.Int32) []*client {

	cstr := func() (int32, error) {
		log.Info("refreshing")
		time.Sleep(time.Second)
		res := seq.Add(1)
		log.Info("refreshed", "new", res)
		return res, nil
	}

	start := make(chan struct{})
	stop := make(chan struct{})
	wg := &sync.WaitGroup{}

	clients := make([]*client, 6)
	for i := 0; i < len(clients); i++ {
		wg.Add(1)
		cl := &client{
			name:    fmt.Sprintf("client-%d", i),
			key:     fmt.Sprintf("key-%d", i/3),
			cache:   cache,
			cstr:    cstr,
			collect: new(assert.CollectT),
		}
		go func() {
			cl.run(start, stop)
			wg.Done()
		}()

		clients[i] = cl
	}

	log.Info("starting")
	close(start)
	time.Sleep(5*time.Second + 500*time.Millisecond)
	close(stop)
	log.Info("stopping")
	wg.Wait()
	log.Info("stopped")

	return clients
}

type client struct {
	name    string
	key     string
	cache   *lru.TypedExpiringCache[string, int32]
	cstr    func() (int32, error)
	collect *assert.CollectT

	previousVal int32
	invocations int
	longWaits   int
	waitTotal   time.Duration
}

func (c *client) assert(t *testing.T) {
	c.collect.Copy(t)
}

func (c *client) callCache() {
	res, evicted, err := c.cache.GetOrCreate(c.key, c.cstr)
	require.NoError(c.collect, err)
	require.False(c.collect, evicted) // never evicted, always refreshed
	require.GreaterOrEqual(c.collect, res, c.previousVal)
	c.previousVal = res
}

func (c *client) run(start chan struct{}, stop chan struct{}) {
	<-start

	ticker := time.NewTicker(10 * time.Millisecond)
	watch := timeutil.StartWatch()
	for j := 0; ; j++ {
		select {
		case <-stop:
			return
		case <-ticker.C:
			watch.Reset()
			c.callCache()
			watch.Stop()
			if watch.Duration() > 10*time.Millisecond {
				c.longWaits++
				log.Info("long wait", "client", c.name, "iteration", j, "duration", watch.Duration(), "count", c.longWaits)
			}
			c.invocations++
			c.waitTotal += watch.Duration()
		}
	}
}

func (c *client) avgWait() time.Duration {
	return c.waitTotal / time.Duration(c.invocations)
}
