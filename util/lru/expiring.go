package lru

import (
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/traceutil"
	"github.com/eluv-io/utc-go"
)

// NewExpiringCache creates a new ExpiringCache.
func NewExpiringCache(maxSize int, maxAge duration.Spec) *ExpiringCache {
	return NewTypedExpiringCache[any, any](maxSize, maxAge)
}

// NewTypedExpiringCache creates a new TypedExpiringCache.
func NewTypedExpiringCache[K any, V any](maxSize int, maxAge duration.Spec) *TypedExpiringCache[K, V] {
	cache := TypedNil[K, *expiringEntry[V]]()
	if maxSize > 0 && maxAge > 0 {
		cache = NewTyped[K, *expiringEntry[V]](maxSize)
	}
	res := &TypedExpiringCache[K, V]{
		cache:  cache,
		maxAge: maxAge.Duration(),
	}
	res.cache.WithMaxAge(maxAge)
	return res
}

// ExpiringCache is an LRU cache that evicts entries from the cache when they reach the configured max age. Expired
// entries are evicted lazily, and hence not garbage collected otherwise. Eviction occurs when
//   - requesting an expired entry through Get, GetOrCreate, Remove
//   - calling Len or Entries
type ExpiringCache = TypedExpiringCache[any, any]

// TypedExpiringCache is a typed LRU cache that evicts entries from the cache when they reach the configured max age.
// Expired entries are evicted lazily, and hence not garbage collected otherwise. Eviction occurs when
//   - requesting an expired entry through Get, GetOrCreate, Remove
//   - calling Len or Entries
type TypedExpiringCache[K any, V any] struct {
	cache                 *TypedCache[K, *expiringEntry[V]]
	maxAge                time.Duration
	resetAgeOnAccess      bool // if true, resets the entries age to 0 on access
	resetAgeAfterCreation bool // if true, resets the entries age to 0 after creation (as opposed to creation start time)
}

// WithMode sets the cache's construction mode.
func (c *TypedExpiringCache[K, V]) WithMode(mode ConstructionMode) *TypedExpiringCache[K, V] {
	c.cache.WithMode(mode)
	return c
}

// WithResetAgeOnAccess turns resetting of an entry's age on access on or off.
func (c *TypedExpiringCache[K, V]) WithResetAgeOnAccess(set bool) *TypedExpiringCache[K, V] {
	c.resetAgeOnAccess = set
	return c
}

// WithResetAgeAfterCreation enables resetting an entry's age after it has been (re-)created. By default, the entry's
// age is based on the start of the creation process - with this option enabled, the construction time is deducted from
// the age.
func (c *TypedExpiringCache[K, V]) WithResetAgeAfterCreation(set bool) *TypedExpiringCache[K, V] {
	c.resetAgeAfterCreation = set
	return c
}

// WithEvictHandler sets the given evict handler.
func (c *TypedExpiringCache[K, V]) WithEvictHandler(onEvicted func(key K, value ExpiringEntry[V])) *TypedExpiringCache[K, V] {
	c.cache.WithEvictHandler(func(key K, value *expiringEntry[V]) {
		onEvicted(key, value)
	})
	return c
}

// WithName sets the cache's name and returns itself for call chaining.
func (c *TypedExpiringCache[K, V]) WithName(name string) *TypedExpiringCache[K, V] {
	if c == nil {
		return nil
	}
	c.cache.WithName(name)
	return c
}

// GetOrCreate looks up a key's value from the cache, creating it if necessary.
//
//   - If the key does not exist, the given constructor function is called to
//     create a new value, store it at the key and return it. If the constructor
//     fails, no value is added to the cache and the error is returned.
//     Otherwise, the new value is added to the cache, and a boolean to mark any
//     evictions from the cache is returned as defined in the Add() method.
//   - If the key exists but is expired according to the max age, the current
//     value is discarded and re-created with the constructor function.
//   - If evict functions are passed and a non-expired cache entry exists, then
//     the first evict function is invoked with the cached value. If it returns
//     true, the value is discarded from the cache and the constructor is called.
func (c *TypedExpiringCache[K, V]) GetOrCreate(
	key K,
	constructor func() (V, error),
	evict ...func(val ExpiringEntry[V]) bool) (val V, evicted bool, err error) {

	now := utc.Now()
	var entry *expiringEntry[V]
	entry, evicted, err = c.cache.GetOrCreate(
		key,
		func() (*expiringEntry[V], error) {
			val, err := constructor()
			if err != nil {
				return nil, err
			}
			ts := now
			if c.resetAgeAfterCreation {
				ts = utc.Now()
			}
			return &expiringEntry[V]{
				val: val,
				ts:  ts,
			}, nil
		},
		c.checkAge(now, evict...),
	)
	if err != nil {
		var zero V
		return zero, evicted, err
	}
	return entry.val, evicted, nil
}

func (c *TypedExpiringCache[K, V]) checkAge(now utc.UTC, evict ...func(val ExpiringEntry[V]) bool) func(val *expiringEntry[V]) bool {
	return func(val *expiringEntry[V]) bool {
		if c.isExpired(val, now) {
			return true
		}
		if c.resetAgeOnAccess {
			val.ts = now
		}
		if len(evict) > 0 {
			return evict[0](val)
		}
		return false
	}
}

func (c *TypedExpiringCache[K, V]) isExpired(val *expiringEntry[V], now utc.UTC) bool {
	age := now.Sub(val.ts)
	if age >= c.maxAge {
		traceutil.Span().Attribute("expired_entry_age", duration.Spec(age).RoundTo(1))
		return true
	}
	return false
}

// Get returns the value stored for the given key and true if the key exists,
// nil and false if the key does not exist or has expired.
func (c *TypedExpiringCache[K, V]) Get(key K) (V, bool) {
	val, ok := c.cache.getOrEvict(key, true, c.checkAge(utc.Now()), nil)
	if ok {
		return val.val, true
	}
	var zero V
	return zero, false
}

// Add adds a value to the cache.  Returns true if an eviction occurred.
func (c *TypedExpiringCache[K, V]) Add(key K, value V) bool {
	return c.cache.Add(key, &expiringEntry[V]{
		val: value,
		ts:  utc.Now(),
	})
}

// Update updates the existing value for the given key or adds it to the cache if it doesn't exist.
// It returns two booleans:
//   - new: true if the key is new, false if it already existed and the entry was updated
//   - evicted: true if an eviction occurred.
func (c *TypedExpiringCache[K, V]) Update(key K, value V) (new bool, evicted bool) {
	return c.cache.UpdateFn(key, func(entry *expiringEntry[V]) *expiringEntry[V] {
		if entry != nil {
			entry.val = value
			entry.ts = utc.Now()
			return entry
		}

		return &expiringEntry[V]{
			val: value,
			ts:  utc.Now(),
		}
	})
}

// Remove removes the entry with the given key.
func (c *TypedExpiringCache[K, V]) Remove(key K) {
	c.cache.Remove(key)
}

// Len returns the number of entries in the cache after evicting any expired entries.
func (c *TypedExpiringCache[K, V]) Len() (len int) {
	now := utc.Now()
	c.cache.runWithWriteLock(func() {
		c.evictExpired(now)
		len = c.cache.lru.Len()
	})
	return len
}

// Entries returns all (non-expired) entries in the cache from oldest to newest.
func (c *TypedExpiringCache[K, V]) Entries() (entries []ExpiringEntry[V]) {
	now := utc.Now()
	c.cache.runWithWriteLock(func() {
		c.evictExpired(now)
		keys := c.cache.lru.Keys()
		entries = make([]ExpiringEntry[V], len(keys))
		for idx, key := range keys {
			val, ok := c.cache.lru.Get(key)
			if ok {
				ee := val.(*expiringEntry[V])
				clone := *ee
				entries[idx] = &clone
			}
		}
	})
	return entries
}

// Purge removes all entries from the cache.
func (c *TypedExpiringCache[K, V]) Purge() {
	c.cache.Purge()
}

// EvictExpired removes all expired entries.
func (c *TypedExpiringCache[K, V]) EvictExpired() {
	c.Len()
}

// evictExpired removes all expired entries
func (c *TypedExpiringCache[K, V]) evictExpired(now utc.UTC) {
	for {
		_, val, ok := c.cache.lru.GetOldest()
		if !ok {
			return
		}
		if c.isExpired(val.(*expiringEntry[V]), now) {
			c.cache.lru.RemoveOldest()
		} else {
			return
		}
	}
}

// Metrics returns a copy of the cache's runtime properties.
func (c *TypedExpiringCache[K, V]) Metrics() Metrics {
	return c.cache.Metrics()
}

// CollectMetrics returns a copy of the cache's runtime properties.
func (c *TypedExpiringCache[K, V]) CollectMetrics() jsonutil.GenericMarshaler {
	m := c.Metrics()
	return &m
}

type ExpiringEntry[V any] interface {
	Value() V
	LastUpdated() utc.UTC
}

type expiringEntry[V any] struct {
	val V
	ts  utc.UTC
}

func (e *expiringEntry[V]) Value() V {
	return e.val
}

func (e *expiringEntry[V]) LastUpdated() utc.UTC {
	return e.ts
}
