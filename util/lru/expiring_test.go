package lru_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/goutil"
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

func TestExpiringCacheResetAgeAfterCreation(t *testing.T) {
	t.Parallel()

	cache := lru.NewExpiringCache(10, duration.Spec(1*time.Second)).WithResetAgeAfterCreation(true)

	cstr := func(v interface{}) func() (interface{}, error) {
		return func() (interface{}, error) {
			time.Sleep(time.Second)
			return v, nil
		}
	}
	assertGoC := func(key interface{}, constructor func() (interface{}, error), wantVal interface{}, wantEviction bool) {
		res, evicted, err := cache.GetOrCreate(key, constructor)
		require.NoError(t, err)
		require.Equal(t, wantVal, res)
		require.Equal(t, wantEviction, evicted)
	}

	stop := make(chan struct{})
	wg := &sync.WaitGroup{}

	assertGoC("k1", cstr("v1"), "v1", false)

	clients := make([]*client, 5)
	for i := 0; i < len(clients); i++ {
		wg.Add(1)
		cl := &client{}
		go func() {
			cl.run(stop, func() {
				assertGoC("k1", cstr("v1"), "v1", false)
			})
			wg.Done()
		}()

		clients[i] = cl
	}

	time.Sleep(5 * time.Second)
	close(stop)
	wg.Wait()

	for _, cl := range clients {
		avgWait := cl.avgWait()
		log.Info("stats",
			"client", cl.Name(),
			"invocations", cl.invocations,
			"long_waits", cl.longWaits,
			"avg_wait", avgWait,
			"total_wait", cl.waitTotal)
		require.Less(t, avgWait, 10*time.Millisecond)
		require.InDelta(t, 2*time.Second, cl.waitTotal, float64(50*time.Millisecond))
	}
}

type client struct {
	invocations int
	longWaits   int
	waitTotal   time.Duration
}

func (c *client) run(stop chan struct{}, call func()) {
	watch := timeutil.StartWatch()
	for j := 0; ; j++ {
		select {
		case <-stop:
			return
		default:
			watch.Reset()
			call()
			watch.Stop()
			if watch.Duration() > 10*time.Millisecond {
				c.longWaits++
				log.Info("long wait", "iteration", j, "duration", watch.Duration(), "count", c.longWaits)
			}
			c.invocations++
			c.waitTotal += watch.Duration()
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func (c *client) avgWait() time.Duration {
	return c.waitTotal / time.Duration(c.invocations)
}

func (c *client) Name() string {
	return fmt.Sprintf("client-%d", goutil.GoID())
}
