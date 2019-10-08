package lru

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"eluvio/log"
	"eluvio/util/syncutil"

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
				val, evicted, err := lru.GetOrCreate(key, createConstructor(key))
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
				val, evicted, err := lru.GetOrCreate(key, createConstructor(key))
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

func TestGetOrCreateConcurrent(t *testing.T) {
	// log.Get("/").SetDebug()

	evictedCount := 0
	lru := NewWithEvict(2, func(key interface{}, value interface{}) {
		evictedCount++
	})

	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("k%d", i)

		wg := &sync.WaitGroup{}
		for j := 0; j < 20; j++ {
			msg := fmt.Sprintf("i=%d, j=%d", i, j)
			wg.Add(1)
			go func() {
				log.Debug("loop", "counters", msg)
				val, _, err := lru.GetOrCreate(key, createConstructor(key))
				require.NoError(t, err)
				require.Equal(t, key, val)
				wg.Done()
			}()
		}
		syncutil.WaitTimeout(wg, time.Second*2)
	}

}

func createConstructor(key string) func() (interface{}, error) {
	return func() (interface{}, error) {
		return key, nil
	}
}
