package syncutil

import (
	"hash/maphash"
	"sync"
	"sync/atomic"
	"unsafe"
)

// DefaultKeyedLockShards is the number of shards of a KeyedLocks whose Shards field is zero. Each shard has its own
// mutex and map, so goroutines that lock different keys rarely contend on the same shard.
const DefaultKeyedLockShards = 32

// keyedLockShardSize is the size of a shard, the cache line size of most CPUs.
const keyedLockShardSize = 64

// keyedLockSeed seeds the hash that picks a key's shard. A zero maphash.Seed is invalid, so the zero value of
// KeyedLocks relies on this package-level seed.
var keyedLockSeed = maphash.MakeSeed()

// KeyedLocks provides locks identified by a key. A lock is created when its key is first locked and released when
// no goroutine holds or waits for it anymore. Locking a key that is in use returns the same lock, which guarantees
// mutual exclusion per key. A goroutine never waits for the holder of another key. Keys in the same shard only share
// the shard's mutex for the brief lookup of their lock.
//
// Each shard keeps its released locks for reuse, as many as it held at once. Locking and unlocking then allocate
// nothing in a steady state, unless more keys than ever before are locked at once. The shards are allocated on the
// first Lock, at 64 bytes each, so an unused KeyedLocks costs only its own two words. A short-lived instance that sees
// little parallel locking should set Shards to 1.
//
// The zero value for KeyedLocks is ready to use. A KeyedLocks must not be copied after first use.
//
// Usage:
//
//	var kl KeyedLocks[string]
//
//	l := kl.Lock("blub")
//	defer l.Unlock()
//	...
type KeyedLocks[K comparable] struct {
	// Shards is the number of shards. Zero means DefaultKeyedLockShards. It is read on the first Lock, so set it
	// before.
	Shards int

	shards atomic.Pointer[[]keyedLockShard[K]]
}

// keyedLockShard holds the locks of the keys that hash to it. It is padded to 64 bytes. The shards need not start at a
// cache line, but the mutexes of neighbouring shards are then still 64 bytes apart and never share a 64-byte line.
type keyedLockShard[K comparable] struct {
	mu    sync.Mutex
	named map[K]*keyedLock[K]
	// free is a list of released locks for reuse, linked through their next field.
	free *keyedLock[K]
	_    [keyedLockShardSize - unsafe.Sizeof(keyedLockShardFields{})]byte
}

// keyedLockShardFields has the layout of the fields of keyedLockShard, whose size is not a constant since it is
// generic. It gives the padding that makes a shard 64 bytes on 32-bit and 64-bit platforms.
type keyedLockShardFields struct {
	mu    sync.Mutex
	named map[int]*int
	free  *int
}

// The build fails if a shard is not 64 bytes, for example when keyedLockShardFields misses a field of keyedLockShard.
var _ = [1]struct{}{}[unsafe.Sizeof(keyedLockShard[string]{})-keyedLockShardSize]

// keyedLock is the lock of one key. Its fields other than mutex are guarded by its shard's mutex.
type keyedLock[K comparable] struct {
	mutex sync.Mutex
	// waiters counts the goroutines beyond the first that hold or wait for the lock.
	waiters int
	key     K
	shard   *keyedLockShard[K]
	next    *keyedLock[K]
}

// Lock gets or creates the lock for the given key, locks it and returns its Unlocker. Call Unlock exactly once:
// locks are reused, so a second call could release the lock of another key.
func (n *KeyedLocks[K]) Lock(key K) Unlocker {
	s := n.shard(key)
	s.mu.Lock()
	if s.named == nil {
		s.named = make(map[K]*keyedLock[K])
	}
	l, found := s.named[key]
	if found {
		l.waiters++
	} else {
		l = s.take()
		l.key = key
		s.named[key] = l
	}
	s.mu.Unlock()

	// A lock just taken from the free list may still be locked by its previous holder, which unlocks it right after
	// releasing the shard. The wait here is that brief.
	l.mutex.Lock()
	return l
}

// shard returns the shard of key, allocating the shards on first use. A single shard needs no hash.
func (n *KeyedLocks[K]) shard(key K) *keyedLockShard[K] {
	shards := n.shards.Load()
	if shards == nil {
		count := n.Shards
		if count <= 0 {
			count = DefaultKeyedLockShards
		}
		created := make([]keyedLockShard[K], count)
		shards = &created
		if !n.shards.CompareAndSwap(nil, shards) {
			shards = n.shards.Load()
		}
	}
	s := *shards
	if len(s) == 1 {
		return &s[0]
	}
	return &s[maphash.Comparable(keyedLockSeed, key)%uint64(len(s))]
}

// len returns the number of locks held or waited for.
func (n *KeyedLocks[K]) len() int {
	shards := n.shards.Load()
	if shards == nil {
		return 0
	}
	var ret int
	for i := range *shards {
		s := &(*shards)[i]
		s.mu.Lock()
		ret += len(s.named)
		s.mu.Unlock()
	}
	return ret
}

// take returns a released lock of the shard for reuse, or a new one. The caller holds the shard's mutex.
func (s *keyedLockShard[K]) take() *keyedLock[K] {
	l := s.free
	if l == nil {
		return &keyedLock[K]{shard: s}
	}
	s.free = l.next
	l.next = nil
	return l
}

// Unlock releases the lock. The last goroutine to release it removes it from its shard and keeps it for reuse.
func (l *keyedLock[K]) Unlock() {
	s := l.shard
	s.mu.Lock()
	if l.waiters > 0 {
		l.waiters--
	} else {
		delete(s.named, l.key)
		var zero K
		l.key = zero // do not keep the key alive
		l.next = s.free
		s.free = l
	}
	s.mu.Unlock()

	// Nothing of l is touched after this, since a goroutine may already have taken it for another key.
	l.mutex.Unlock()
}
