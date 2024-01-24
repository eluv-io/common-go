package lru

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"go.uber.org/atomic"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/jsonutil"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

var modes = []ConstructionMode{Modes.Blocking, Modes.Concurrent, Modes.Decoupled}

func TestBasic(t *testing.T) {
	evictedCount := 0
	lru := NewWithEvict(1, func(key interface{}, value interface{}) {
		evictedCount++
	}).WithName("test-cache")

	assertMetrics := createAssertMetricsFn(t, lru, "test-cache")

	key1 := "key"
	val1 := "val1"
	key2 := "key2"
	val2 := "val2"
	key3 := "key3"
	val3 := "val3"

	val, evicted := lru.Get(key1)
	require.Nil(t, val)
	require.False(t, evicted)
	assertMetrics(0, 1, 0, 0)

	val, evicted = lru.Get(key1)
	require.Nil(t, val)
	require.False(t, evicted)
	assertMetrics(0, 2, 0, 0)

	// add key1
	evicted = lru.Add(key1, val1)
	require.False(t, evicted)
	assertMetrics(0, 2, 1, 0)

	var ok bool
	val, ok = lru.Get(key1)
	require.True(t, ok)
	require.Equal(t, val1, val)
	assertMetrics(1, 2, 1, 0)

	val, ok = lru.Get(key2)
	require.False(t, ok)
	require.Nil(t, val)
	assertMetrics(1, 3, 1, 0)

	// add key2
	evicted = lru.Add(key2, val2)
	require.True(t, evicted)
	assertMetrics(1, 3, 2, 1)

	val, ok = lru.Get(key1)
	require.False(t, ok)
	require.Nil(t, val)
	assertMetrics(1, 4, 2, 1)

	val, ok = lru.Get(key2)
	require.True(t, ok)
	require.Equal(t, val2, val)
	assertMetrics(2, 4, 2, 1)

	// add key2 again
	val2 = "val2-2"
	evicted = lru.Add(key2, val2)
	require.False(t, evicted)
	assertMetrics(2, 4, 3, 2) // no eviction, but still removed and added!

	// update existing key
	var isNew bool
	isNew, evicted = lru.Update(key2, val2)
	require.False(t, isNew)
	require.False(t, evicted)
	assertMetrics(2, 4, 4, 3) // no eviction, but still removed and added!

	val, ok = lru.Peek(key2)
	require.True(t, ok)
	require.Equal(t, val2, val)

	// update existing key with UpdateFn
	isNew, evicted = lru.UpdateFn(key2, func(value interface{}) interface{} {
		require.Equal(t, val2, value)
		val2 = "val2-3"
		return val2
	})
	require.False(t, isNew)
	require.False(t, evicted)
	assertMetrics(2, 4, 5, 4) // no eviction, but still removed and added!

	val, ok = lru.Peek(key2)
	require.True(t, ok)
	require.Equal(t, val2, val)

	// update non-existing key with UpdateFn
	isNew, evicted = lru.UpdateFn(key3, func(value interface{}) interface{} {
		require.Nil(t, value)
		return val3
	})
	require.True(t, isNew)
	require.True(t, evicted)
	assertMetrics(2, 4, 6, 5)

	val, ok = lru.Peek(key3)
	require.True(t, ok)
	require.Equal(t, val3, val)
}

func TestGetOrCreateBasic(t *testing.T) {
	for _, mode := range modes {
		t.Run(fmt.Sprintf("%s-mode", mode), func(t *testing.T) {

			evictedCount := 0
			lru := NewWithEvict(1, func(key interface{}, value interface{}) {
				evictedCount++
			})
			assertMetrics := createAssertMetricsFn(t, lru, "")

			assertMetrics(0, 0, 0, 0)

			key1 := "key"
			cstr1 := constructor(key1, 0)
			key2 := "key2"
			cstr2 := constructor(key2, 0)

			val, evicted, err := lru.GetOrCreate(key1, cstr1)
			require.NoError(t, err)
			require.False(t, evicted)
			require.Equal(t, key1, val)
			require.Equal(t, 0, evictedCount)
			assertMetrics(0, 1, 1, 0)

			val, evicted, err = lru.GetOrCreate(key1, cstr1)
			require.NoError(t, err)
			require.False(t, evicted)
			require.Equal(t, key1, val)
			require.Equal(t, 0, evictedCount)
			assertMetrics(1, 1, 1, 0)

			val, evicted, err = lru.GetOrCreate(key1, cstr1)
			require.NoError(t, err)
			require.False(t, evicted)
			require.Equal(t, key1, val)
			require.Equal(t, 0, evictedCount)
			assertMetrics(2, 1, 1, 0)

			// now insert a new key, which should trigger eviction of key1
			val, evicted, err = lru.GetOrCreate(key2, cstr2)
			require.NoError(t, err)
			require.True(t, evicted)
			require.Equal(t, key2, val)
			require.Equal(t, 1, evictedCount)
			assertMetrics(2, 2, 2, 1)

			// requesting key1 switches back
			val, evicted, err = lru.GetOrCreate(key1, cstr1)
			require.NoError(t, err)
			require.True(t, evicted)
			require.Equal(t, key1, val)
			require.Equal(t, 2, evictedCount)
			assertMetrics(2, 3, 3, 2)

			// now replace key1 due to staleness
			val, evicted, err = lru.GetOrCreate(key1, cstr1, func(val interface{}) bool {
				return true
			})
			require.NoError(t, err)
			require.False(t, evicted)
			require.Equal(t, key1, val)
			require.Equal(t, 3, evictedCount) // evicted count does change, since the entry was recreated
			assertMetrics(2, 4, 4, 4)         // but the entry was nevertheless removed (and re-added)

		})
	}
}

func createAssertMetricsFn(t *testing.T, lru *Cache, name string) func(hits int, misses int, added int, removed int) {
	return func(hits, misses, added, removed int) {
		m := lru.Metrics()
		require.Equal(t, name, m.Name)
		require.EqualValues(t, hits, m.Hits.Load(), "hits")
		require.EqualValues(t, misses, m.Misses.Load(), "misses")
		require.EqualValues(t, added, m.Added.Load(), "added")
		require.EqualValues(t, removed, m.Removed.Load(), "removed")
	}
}

func TestGetOrCreate(t *testing.T) {
	Convey("Given an LRU cache of size 2", t, func() {
		evictedCount := 0
		lru := NewWithEvict(2, func(key interface{}, value interface{}) {
			evictedCount++
		})

		Convey("GetOrCreate() creates and returns the correct value and evicted flag", func() {
			for i := 0; i < 10; i++ {
				key := fmt.Sprintf("k%d", i)
				val, evicted, err := lru.GetOrCreate(key, constructor(key, 0))
				So(err, ShouldBeNil)
				So(val, ShouldEqual, key)
				metrics := lru.Metrics()
				So(metrics.Hits.Load(), ShouldEqual, 0)
				So(metrics.Misses.Load(), ShouldEqual, i+1)
				So(metrics.Added.Load(), ShouldEqual, i+1)
				if i < 2 {
					So(evicted, ShouldBeFalse)
					So(evictedCount, ShouldEqual, 0)
					So(metrics.Removed.Load(), ShouldEqual, 0)
				} else {
					So(evicted, ShouldBeTrue)
					So(evictedCount, ShouldEqual, i-1)
					So(metrics.Removed.Load(), ShouldEqual, i-1)
				}
			}

			Convey("The remaining elements in the cache are correct", func() {
				val, found := lru.Peek("k8")
				So(found, ShouldBeTrue)
				So(val, ShouldEqual, "k8")

				val, found = lru.Peek("k9")
				So(found, ShouldBeTrue)
				So(val, ShouldEqual, "k9")

				_, found = lru.Peek("k7")
				So(found, ShouldBeFalse)

				_, found = lru.Peek("k10")
				So(found, ShouldBeFalse)

			})
		})
	})
}

func TestNilCache(t *testing.T) {
	Convey("Given a nil LRU cache", t, func() {
		evictedCount := 0
		lru := Nil()

		Convey("GetOrCreate() creates and returns the correct value and evicted flag", func() {
			for i := 0; i < 10; i++ {
				key := fmt.Sprintf("k%d", i)
				val, evicted, err := lru.GetOrCreate(key, constructor(key, 0))
				So(err, ShouldBeNil)
				So(val, ShouldEqual, key)
				So(evicted, ShouldBeFalse)
				So(evictedCount, ShouldEqual, 0)
			}
		})
		key := "key"
		val := "val"
		Convey("Add works", func() {
			So(lru.Add(key, val), ShouldBeFalse)
			Convey("Get returns nil", func() {
				val, evicted := lru.Get(key)
				So(val, ShouldBeNil)
				So(evicted, ShouldBeFalse)
			})
			Convey("Peek returns nil", func() {
				val, found := lru.Peek(key)
				So(val, ShouldBeNil)
				So(found, ShouldBeFalse)
			})
			Convey("Contains returns false", func() {
				So(lru.Contains(key), ShouldBeFalse)
			})
			Convey("Len returns 0", func() {
				So(lru.Len(), ShouldEqual, 0)
			})
			Convey("ContainsOrAdd returns false", func() {
				ok, evicted := lru.ContainsOrAdd(key, val)
				So(ok, ShouldBeFalse)
				So(evicted, ShouldBeFalse)
			})
			Convey("Keys returns empty slice", func() {
				keys := lru.Keys()
				So(keys, ShouldNotBeNil)
				So(len(keys), ShouldEqual, 0)
			})
			Convey("Purge, Remove and RemoveOldest don't crash", func() {
				lru.Purge()
				lru.Remove(key)
				lru.RemoveOldest()
			})
			Convey("Metrics returns an empty struct", func() {
				m := lru.Metrics()
				So(m.Hits.Load(), ShouldEqual, 0)
				So(m.Misses.Load(), ShouldEqual, 0)
				So(m.Errors.Load(), ShouldEqual, 0)
				So(m.Added.Load(), ShouldEqual, 0)
				So(m.Removed.Load(), ShouldEqual, 0)
				So(m.Config.MaxItems, ShouldEqual, 0)
				So(m.Config.MaxAge, ShouldEqual, 0)
				So(m.Config.Mode, ShouldEqual, "")
			})
		})
	})
}

func TestGetOrCreateStress(t *testing.T) {
	// log.Get("/").SetDebug()

	const (
		workers           = 20
		cacheSize         = 10
		keyRange          = 20
		runTime           = "2s"
		constructionDelay = "5ms"
	)
	runDuration := duration.MustParse(runTime).Duration()
	constDelay := duration.MustParse(constructionDelay).Duration()

	for _, mode := range modes {
		t.Run(fmt.Sprintf("%s-mode", mode), func(t *testing.T) {
			lru := New(cacheSize)
			lru.Mode = mode

			keys := make([]string, keyRange)
			for i := 0; i < keyRange; i++ {
				keys[i] = fmt.Sprintf("k%.2d", i)
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
						key := keys[rand.Intn(keyRange)]
						val, evicted, err := lru.GetOrCreate(key, constructor(key, constDelay))
						require.NoError(t, err)
						require.Equal(t, key, val)
						count++
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
			metrics := lru.Metrics()
			fmt.Println(jsonutil.MarshalString(metrics))
			require.EqualValues(t, totalCount, metrics.Hits.Load()+metrics.Misses.Load())
			require.EqualValues(t, 0, metrics.Errors.Load())
			require.EqualValues(t, cacheSize, metrics.Config.MaxItems)
			require.EqualValues(t, cacheSize, metrics.Added.Load()-metrics.Removed.Load())
		})
	}

}

func constructor(key string, sleep time.Duration) func() (interface{}, error) {
	return func() (interface{}, error) {
		if sleep > 0 {
			time.Sleep(sleep)
		}
		return key, nil
	}
}

func TestGetValidOrCreate(t *testing.T) {

	const (
		workers           = 20
		cacheSize         = 10
		keyRange          = 5 // no eviction - test expects keyRange < cacheSize
		runTime           = "2s"
		valid             = "500ms"
		constructionDelay = "5ms"
	)

	rand.Seed(time.Now().UnixNano())

	runDuration := duration.MustParse(runTime).Duration()
	constDelay := duration.MustParse(constructionDelay).Duration()
	validity := duration.MustParse(valid).Duration()

	for _, mode := range modes {
		t.Run(fmt.Sprintf("%s-mode", mode), func(t *testing.T) {
			lru := New(cacheSize)
			lru.Mode = mode

			keys := make([]string, keyRange)
			for i := 0; i < keyRange; i++ {
				keys[i] = fmt.Sprintf("k%.2d", i)
			}

			ctor := &ctor{}
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
						now := time.Now()
						key := keys[rand.Intn(keyRange)]
						val, evicted, err := lru.GetOrCreate(
							key,
							ctor.constructor(key, constDelay),
							func(val interface{}) bool {
								if now.Sub(val.(*testResource).createdAt) < validity {
									return false
								}
								return true // expired
							})
						require.NoError(t, err)
						require.Equal(t, key, val.(*testResource).key)
						count++
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
			require.Equal(t, 0, evictedCount)
			if mode != Modes.Concurrent {
				require.Equal(
					t,
					int64(keyRange*(runDuration/validity)),
					ctor.invoked.Load())
			}

			fmt.Println("total", totalCount, "ctor invoked", ctor.invoked.Load())
		})
	}

}

type ctor struct {
	invoked atomic.Int64
}

type testResource struct {
	key       string
	createdAt time.Time
}

func (c *ctor) constructor(key string, sleep time.Duration) func() (interface{}, error) {
	return func() (interface{}, error) {
		c.invoked.Inc()
		if sleep > 0 {
			time.Sleep(sleep)
		}
		return &testResource{
			key:       key,
			createdAt: time.Now(),
		}, nil
	}
}
