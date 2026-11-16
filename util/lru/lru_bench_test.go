package lru

import (
	"fmt"
	"testing"
)

// testdata/lru_bench.txt holds the results of runs on a 4-core VM, in the output format of go test, one for each
// commit that changed the cache:
//   - "impl: simplelru" is the cache on hashicorp's simplelru, locking keys with NamedLocks.
//   - "impl: simplelru-keyedlocks" locks keys with KeyedLocks[K].
//   - "impl: generic" keeps the entries in the generic simpleLRU of this package.
//
// Show them side by side with benchstat (go install golang.org/x/perf/cmd/benchstat@latest), and compare them with a
// new run from the repository root:
//
//	benchstat -col impl util/lru/testdata/lru_bench.txt
//	echo 'impl: new' > new.txt
//	go test ./util/lru/ -run '^$' -bench TypedCache -benchmem -count 6 -benchtime 200ms >> new.txt
//	benchstat -col impl util/lru/testdata/lru_bench.txt new.txt
//
// Times vary between machines, but the allocations do not.

const benchCacheKeys = 1024

var benchCacheKeyNames = func() []string {
	ret := make([]string, benchCacheKeys)
	for i := range ret {
		ret[i] = fmt.Sprintf("key-%d", i)
	}
	return ret
}()

// BenchmarkTypedCacheDecoupledMiss removes a string key and gets it again from a cache in decoupled mode, so every
// GetOrCreate misses and constructs the value under the key's lock. The lock allocates nothing with KeyedLocks[K],
// while NamedLocks boxed the key of each lock. The entry allocates nothing with simpleLRU, which reuses the removed
// one. hashicorp's simplelru allocated the entry and its list element and boxed the key.
func BenchmarkTypedCacheDecoupledMiss(b *testing.B) {
	c := NewTyped[string, int](benchCacheKeys).WithMode(Modes.Decoupled)
	construct := func() (int, error) { return 1, nil }

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		key := benchCacheKeyNames[i%benchCacheKeys]
		c.Remove(key)
		if _, _, err := c.GetOrCreate(key, construct); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTypedCacheGet gets string keys that are all in the cache.
func BenchmarkTypedCacheGet(b *testing.B) {
	c := NewTyped[string, int](benchCacheKeys)
	for _, key := range benchCacheKeyNames {
		c.Add(key, 1)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, ok := c.Get(benchCacheKeyNames[i%benchCacheKeys]); !ok {
			b.Fatal("missing key")
		}
	}
}

// BenchmarkTypedCacheAddEvict adds string keys to a full cache of half their number, so every Add evicts the oldest
// key. simpleLRU reuses the evicted entry for the next Add.
func BenchmarkTypedCacheAddEvict(b *testing.B) {
	c := NewTyped[string, int](benchCacheKeys / 2)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.Add(benchCacheKeyNames[i%benchCacheKeys], 1)
	}
}
