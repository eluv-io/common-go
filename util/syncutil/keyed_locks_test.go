package syncutil

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestKeyedLocksAllocateNothing: once a shard has released a lock, locking and unlocking a key allocates nothing, for
// a string key and for a struct key. A burst allocates nothing once a burst of the same size has run.
func TestKeyedLocksAllocateNothing(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		var kl KeyedLocks[string]
		kl.Lock("warm-up").Unlock()
		key := "key"
		require.Zero(t, testing.AllocsPerRun(1000, func() { kl.Lock(key).Unlock() }))
	})
	t.Run("one shard", func(t *testing.T) {
		kl := KeyedLocks[string]{Shards: 1}
		kl.Lock("warm-up").Unlock()
		key := "key"
		require.Zero(t, testing.AllocsPerRun(1000, func() { kl.Lock(key).Unlock() }))
	})
	t.Run("struct", func(t *testing.T) {
		type pair struct {
			a string
			b int
		}
		var kl KeyedLocks[pair]
		kl.Lock(pair{"warm-up", 0}).Unlock()
		key := pair{"key", 1}
		require.Zero(t, testing.AllocsPerRun(1000, func() { kl.Lock(key).Unlock() }))
	})
	t.Run("burst", func(t *testing.T) {
		// One shard holds all 64 locks at once, so it must keep all of them for reuse.
		kl := KeyedLocks[string]{Shards: 1}
		names := make([]string, 64)
		for i := range names {
			names[i] = fmt.Sprintf("k%d", i)
		}
		unlockers := make([]Unlocker, len(names))
		burst := func() {
			for i, name := range names {
				unlockers[i] = kl.Lock(name)
			}
			for _, u := range unlockers {
				u.Unlock()
			}
		}
		burst()
		require.Zero(t, testing.AllocsPerRun(100, burst))
	})
}

// TestKeyedLocksAllocateShardsLazily: a KeyedLocks that was never locked has not allocated its shards.
func TestKeyedLocksAllocateShardsLazily(t *testing.T) {
	var kl KeyedLocks[string]
	require.Nil(t, kl.shards.Load())
	require.Zero(t, kl.len())
	kl.Lock("a").Unlock()
	require.NotNil(t, kl.shards.Load())
	require.Zero(t, kl.len())
}

// TestKeyedLocksShards: the first Lock allocates the configured number of shards, the default for zero.
func TestKeyedLocksShards(t *testing.T) {
	for _, tc := range []struct{ configured, want int }{{0, DefaultKeyedLockShards}, {1, 1}, {5, 5}} {
		kl := KeyedLocks[string]{Shards: tc.configured}
		kl.Lock("a").Unlock()
		require.Len(t, *kl.shards.Load(), tc.want, "configured %d", tc.configured)
	}
}

// TestKeyedLocksExclusion: each of a few keys is locked by many goroutines that increment a plain counter under the
// lock. Released locks are reused all the time, so a lock that was released too early shows as a lost increment or as
// a race.
func TestKeyedLocksExclusion(t *testing.T) {
	for _, shards := range []int{0, 1} {
		t.Run(fmt.Sprintf("shards %d", shards), func(t *testing.T) {
			testKeyedLocksExclusion(t, &KeyedLocks[string]{Shards: shards})
		})
	}
}

func testKeyedLocksExclusion(t *testing.T, kl *KeyedLocks[string]) {
	const (
		keys       = 5
		goroutines = 50
		rounds     = 200
	)
	names := make([]string, keys)
	counters := make([]int, keys)
	for i := range names {
		names[i] = fmt.Sprintf("k%d", i)
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				i := (g + r) % keys
				l := kl.Lock(names[i])
				counters[i]++
				l.Unlock()
			}
		}()
	}
	require.False(t, WaitTimeout(&wg, time.Minute))
	for i, c := range counters {
		require.Equal(t, goroutines*rounds/keys, c, "counter of %s", names[i])
	}
	require.Zero(t, kl.len(), "no lock is left once all are released")
}

// TestKeyedLocksDifferentKeys: a held key does not block another key.
func TestKeyedLocksDifferentKeys(t *testing.T) {
	var kl KeyedLocks[int]
	held := kl.Lock(1)
	done := make(chan struct{})
	go func() {
		kl.Lock(2).Unlock()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("locking another key waited for the held one")
	}
	held.Unlock()
	require.Zero(t, kl.len())
}
