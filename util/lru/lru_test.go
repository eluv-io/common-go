package lru

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/qluvio/content-fabric/format/duration"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/require"
)

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
				if i < 2 {
					So(evicted, ShouldBeFalse)
					So(evictedCount, ShouldEqual, 0)
				} else {
					So(evicted, ShouldBeTrue)
					So(evictedCount, ShouldEqual, i-1)
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
		})
	})
}

func TestGetOrCreateStress(t *testing.T) {
	// log.Get("/").SetDebug()
	modes := []constructionMode{Modes.Blocking, Modes.Concurrent, Modes.Decoupled}

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

			require.Empty(t, lru.createLocks)
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
