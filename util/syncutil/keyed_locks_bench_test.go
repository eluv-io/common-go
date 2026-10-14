package syncutil

import (
	"fmt"
	"hash/maphash"
	"sync"
	"sync/atomic"
	"testing"
)

// The benchmarks compare lock implementations, each locking a string key. legacy is the NamedLocks implementation
// before KeyedLocks, named is NamedLocks as KeyedLocks[any], keyed is KeyedLocks[string] with the default shards,
// keyed-1shard has Shards set to 1, and striped is 256 mutexes picked by hash.
//
// testdata/keyed_locks_bench.txt holds the results of a run on a 4-core VM, in the output format of go test. Re-run
// them from the repository root and compare with benchstat (go install golang.org/x/perf/cmd/benchstat@latest):
//
//	go test ./util/syncutil/ -run '^$' -bench KeyedLocks -benchmem -cpu 1,4,16 -count 6 -benchtime 200ms > new.txt
//	benchstat -col /impl util/syncutil/testdata/keyed_locks_bench.txt
//	benchstat -col /impl new.txt
//	benchstat util/syncutil/testdata/keyed_locks_bench.txt new.txt
//
// With -col /impl, benchstat shows the implementations side by side, compared to legacy. The last call compares the
// recorded results with the new run. Times vary between machines, but the allocations do not.

const benchLockKeys = 1024

var benchLockNames = func() []string {
	ret := make([]string, benchLockKeys)
	for i := range ret {
		ret[i] = fmt.Sprintf("key-%d", i)
	}
	return ret
}()

// stripedLocks is a fixed array of mutexes picked by the key's hash, the alternative to per-key locks when keys may
// share a lock.
type stripedLocks [256]sync.Mutex

func (s *stripedLocks) Lock(key string) Unlocker {
	mu := &s[maphash.String(keyedLockSeed, key)%uint64(len(s))]
	mu.Lock()
	return mu
}

// benchLockers are the lock implementations the benchmarks compare.
var benchLockers = []struct {
	name string
	make func() func(key string) Unlocker
}{
	{name: "legacy", make: func() func(string) Unlocker {
		var l legacyNamedLocks
		return func(key string) Unlocker { return l.Lock(key) }
	}},
	{name: "named", make: func() func(string) Unlocker {
		var l NamedLocks
		return func(key string) Unlocker { return l.Lock(key) }
	}},
	{name: "keyed", make: func() func(string) Unlocker {
		var l KeyedLocks[string]
		return func(key string) Unlocker { return l.Lock(key) }
	}},
	{name: "keyed-1shard", make: func() func(string) Unlocker {
		l := KeyedLocks[string]{Shards: 1}
		return func(key string) Unlocker { return l.Lock(key) }
	}},
	{name: "striped", make: func() func(string) Unlocker {
		var l stripedLocks
		return func(key string) Unlocker { return l.Lock(key) }
	}},
}

// BenchmarkKeyedLocksFirstLock creates an instance and locks and unlocks one key in it, which shows what a short-lived
// instance costs, such as the one of a short-lived lru cache. named takes 2648 bytes, most of them for its 32 shards of
// 64 bytes. named-1shard takes 408 bytes, legacy 344.
func BenchmarkKeyedLocksFirstLock(b *testing.B) {
	b.Run("impl=legacy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var l legacyNamedLocks
			l.Lock("key").Unlock()
		}
	})
	b.Run("impl=named", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var l NamedLocks
			l.Lock("key").Unlock()
		}
	})
	b.Run("impl=named-1shard", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			l := NamedLocks{Shards: 1}
			l.Lock("key").Unlock()
		}
	})
}

// BenchmarkKeyedLocksSerial locks and unlocks a rotating key from one goroutine. keyed allocates nothing and takes
// about half the time of legacy, which allocates 3 times. named allocates once, for the boxed key.
func BenchmarkKeyedLocksSerial(b *testing.B) {
	for _, bl := range benchLockers {
		b.Run("impl="+bl.name, func(b *testing.B) {
			lock := bl.make()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				lock(benchLockNames[i%benchLockKeys]).Unlock()
			}
		})
	}
}

// BenchmarkKeyedLocksParallel locks and unlocks rotating keys from parallel goroutines. Each goroutine starts at
// another key, so two goroutines rarely want the same key at once. This measures the cost of the bookkeeping shared by
// all keys. keyed takes the same time as serially, while legacy slows down. keyed-1shard slows down too, since all
// goroutines contend on its one shard mutex.
func BenchmarkKeyedLocksParallel(b *testing.B) {
	for _, bl := range benchLockers {
		b.Run("impl="+bl.name, func(b *testing.B) {
			lock := bl.make()
			var worker atomic.Int64
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				i := int(worker.Add(1)) * (benchLockKeys / 16)
				for pb.Next() {
					lock(benchLockNames[i%benchLockKeys]).Unlock()
					i++
				}
			})
		})
	}
}

// BenchmarkKeyedLocksBurst locks 16 or 64 keys, holds them all at once and unlocks them. keyed keeps all the locks a
// burst released for the next one, even with one shard, so it allocates nothing. striped is left out, since two of the
// keys may share its mutex.
func BenchmarkKeyedLocksBurst(b *testing.B) {
	for _, n := range []int{16, 64} {
		for _, bl := range benchLockers {
			if bl.name == "striped" {
				continue
			}
			b.Run(fmt.Sprintf("keys=%d/impl=%s", n, bl.name), func(b *testing.B) {
				lock := bl.make()
				names := benchLockNames[:n]
				unlockers := make([]Unlocker, n)
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					for j, name := range names {
						unlockers[j] = lock(name)
					}
					for _, u := range unlockers {
						u.Unlock()
					}
				}
			})
		}
	}
}

// BenchmarkKeyedLocksSameKey locks and unlocks one key from parallel goroutines, so they wait for each other.
func BenchmarkKeyedLocksSameKey(b *testing.B) {
	for _, bl := range benchLockers {
		b.Run("impl="+bl.name, func(b *testing.B) {
			lock := bl.make()
			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					lock("hot").Unlock()
				}
			})
		})
	}
}
