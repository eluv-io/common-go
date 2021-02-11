// The lru package provides a simple LRU cache. It is a small adaption of the
// LRU implementation of github.com/hashicorp/golang-lru/lru.go that adds the
// function GetOrCreate() to atomically get a cached value or create it if it
// does not yet exists in the cache.
package lru

import (
	"sync"

	"github.com/hashicorp/golang-lru/simplelru"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/duration"
	"github.com/qluvio/content-fabric/util/jsonutil"
	"github.com/qluvio/content-fabric/util/syncutil"
)

type ConstructionMode string

// Modes defines the different construction modes that can be used with the LRU
// cache. The mode affects the synchronization in calls to the GetOrCreate(key)
// method.
//
// * Blocking: the write lock of the cache is held during the entire lookup and
// creation phase. This means that calls to the constructor are mutually
// exclusive and block all operations of the cache until completed.
//
// * Concurrent: the write lock of the cache is released before the constructor
// is called. It is therefore possible that the constructor for the same key is
// called concurrently. This mode provides maximum concurrency.
//
// * Decoupled: the write lock of the cache is released before the constructor
// is called. However, concurrent calls to the constructor with the same key are
// prevented by acquiring key-specific locks. This mode provides concurrency
// among different keys, but prevents it for a given key.
var Modes = struct {
	Blocking   ConstructionMode
	Concurrent ConstructionMode
	Decoupled  ConstructionMode
}{
	Blocking:   "blocking",
	Concurrent: "concurrent",
	Decoupled:  "decoupled",
}

// Cache is a thread-safe fixed size LRU cache.
type Cache struct {
	lru          *simplelru.LRU
	lock         sync.RWMutex
	Mode         ConstructionMode // defaults to Blocking...
	namedLocks   syncutil.NamedLocks
	metrics      Metrics
	evictHandler func(key interface{}, value interface{}) // the external evict handler function
}

// Nil creates a cache that doesn't cache anything at all.
func Nil() *Cache {
	return nil
}

// New creates an LRU cache of the given size. The size is set to 1 if <= 0
func New(size int) *Cache {
	return NewWithEvict(size, nil)
}

// NewWithEvict constructs a fixed size cache with the given eviction
// callback.
func NewWithEvict(size int, onEvicted func(key interface{}, value interface{})) *Cache {
	if size <= 0 {
		return Nil()
	}
	c := &Cache{
		evictHandler: onEvicted,
	}
	c.lru, _ = simplelru.NewLRU(size, c.onEvict)
	c.metrics.Config.MaxItems = size
	c.WithMode(Modes.Blocking)
	return c
}

// WithMode sets the cache's construction mode and returns itself for call
// chaining.
func (c *Cache) WithMode(mode ConstructionMode) *Cache {
	if c == nil {
		return nil
	}
	c.Mode = mode
	c.metrics.Config.Mode = string(mode)
	return c
}

// WithName sets the cache's name and returns itself for call chaining.
func (c *Cache) WithName(name string) *Cache {
	if c == nil {
		return nil
	}
	c.metrics.Name = name
	return c
}

// WithMaxAge sets the cache's maxAge (only for collection in metrics) and
// returns itself for call chaining.
func (c *Cache) WithMaxAge(age duration.Spec) *Cache {
	if c == nil {
		return nil
	}
	c.metrics.Config.MaxAge = age
	return c
}

// Purge is used to completely clear the cache
func (c *Cache) Purge() {
	if c == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	c.lru.Purge()
}

// Add adds a value to the cache.  Returns true if an eviction occurred.
func (c *Cache) Add(key, value interface{}) bool {
	if c == nil {
		return false
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	c.metrics.Add()
	if c.lru.Contains(key) {
		// updating existing key is basically a remove and an add. Since simple
		// lru doesn't count this as an eviction (and does not call the evict
		// callback), we have to update the metrics here...
		c.metrics.Remove()
	}
	return c.lru.Add(key, value)
}

// Get looks up a key's value from the cache.
func (c *Cache) Get(key interface{}) (interface{}, bool) {
	if c == nil {
		return nil, false
	}
	// need the write lock, since this updates the recently-used list in
	// simple.LRU!
	c.lock.Lock()
	defer c.lock.Unlock()

	res, found := c.lru.Get(key)
	if found {
		c.metrics.Hit()
	} else {
		c.metrics.Miss()
	}
	return res, found
}

// GetOrCreate looks up a key's value from the cache, creating it if necessary.
// Invalid, stale or expired entries are discarded from the cache as dictated
// by the first optional evict parameter.
// - If the key does not exist, the given constructor function is called to
//   create a new value, store it at the key and return it. If the constructor
//   fails, no value is added to the cache and the error is returned. Otherwise,
//   the new value is added to the cache, and a boolean to mark any evictions
//   from the cache is returned as defined in the Add() method.
// - If the key exists and the evict parameter is not nil, then the first evict
//   function is invoked with the retrieved value. If it returns true, the
//   value is discarded from the cache and the constructor is called.
func (c *Cache) GetOrCreate(
	key interface{},
	constructor func() (interface{}, error),
	evict ...func(val interface{}) bool) (val interface{}, evicted bool, err error) {

	if c == nil {
		val, err = constructor()
		return val, false, err
	}

	var evictFn func(val interface{}) bool
	if len(evict) > 0 {
		evictFn = evict[0]
	}

	switch c.Mode {
	case Modes.Blocking:
		return c.getOrCreateBlocking(key, constructor, evictFn)
	case Modes.Decoupled:
		return c.getOrCreateDecoupled(key, constructor, evictFn)
	case Modes.Concurrent:
		return c.getOrCreateConcurrent(key, constructor, evictFn)
	}

	// should never get here!
	return nil, false, errors.E("cache.GetValidOrCreate", errors.K.Invalid, "reason", "invalid construction mode", "mode", c.Mode)
}

func (c *Cache) getOrCreateBlocking(
	key interface{},
	constructor func() (interface{}, error),
	evict func(val interface{}) bool) (val interface{}, evicted bool, err error) {

	c.lock.Lock()
	defer c.lock.Unlock()

	var ok bool
	val, ok = c.getOrEvict(key, false, evict, nil)
	if ok {
		return val, false, nil
	}

	// create the value - holding the write lock (and blocking any other call...)
	val, err = constructor()
	if err != nil {
		c.metrics.Error()
		return nil, false, err
	}
	c.metrics.Add()
	evicted = c.lru.Add(key, val)
	return val, evicted, err
}

func (c *Cache) getOrCreateDecoupled(
	key interface{},
	constructor func() (interface{}, error),
	evict func(val interface{}) bool) (val interface{}, evicted bool, err error) {

	// try to get the value with regular rw lock
	var ok bool
	val, ok = c.getOrEvict(key, true, evict, nil)
	if ok {
		return val, false, nil
	}

	// get the creation mutex for this key
	keyMutex := c.namedLocks.Lock(key)
	defer keyMutex.Unlock()

	// try getting the value again - it might have been created in the meantime
	val, ok = c.getOrEvict(key, true, evict, func() {
		// decrement the metrics' Misses count - independent of the new result: if
		// we still have a miss, we counted them twice. If we get a hit now, the
		// previous miss was incorrect.
		c.metrics.UnMiss()
	})

	if ok {
		return val, false, nil
	}

	// create the value
	val, err = constructor()
	if err != nil {
		c.metrics.Error()
		return nil, false, err
	}

	// add it to the cache (using the regular rw lock)
	evicted = c.Add(key, val)
	return val, evicted, nil
}

func (c *Cache) getOrCreateConcurrent(
	key interface{},
	constructor func() (interface{}, error),
	evict func(val interface{}) bool) (val interface{}, evicted bool, err error) {

	// try to get the value with regular rw lock
	var ok bool
	val, ok = c.getOrEvict(key, true, evict, nil)
	if ok {
		return val, false, nil
	}

	// create the value - holding no lock at all
	val, err = constructor()
	if err != nil {
		c.lock.Lock()
		c.metrics.Error()
		c.lock.Unlock()
		return nil, false, err
	}

	// add it to the cache
	evicted = c.Add(key, val)
	return val, evicted, err
}

// Check if a key is in the cache, without updating the recent-ness
// or deleting it for being stale.
func (c *Cache) Contains(key interface{}) bool {
	if c == nil {
		return false
	}
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.lru.Contains(key)
}

// Returns the key value (or undefined if not found) without updating
// the "recently used"-ness of the key.
func (c *Cache) Peek(key interface{}) (interface{}, bool) {
	if c == nil {
		return nil, false
	}
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.lru.Peek(key)
}

// ContainsOrAdd checks if a key is in the cache without updating the
// recent-ness or deleting it for being stale, and if not, adds the value.
// Returns whether found and whether an eviction occurred.
func (c *Cache) ContainsOrAdd(key, value interface{}) (ok, evict bool) {
	if c == nil {
		return false, false
	}
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.lru.Contains(key) {
		return true, false
	} else {
		c.metrics.Add()
		evict = c.lru.Add(key, value)
		return false, evict
	}
}

// Remove removes the provided key from the cache.
func (c *Cache) Remove(key interface{}) {
	if c == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	// no need to update metrics: lru.Remove calls onEvicted()
	c.lru.Remove(key)
}

// RemoveOldest removes the oldest item from the cache.
func (c *Cache) RemoveOldest() {
	if c == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	// no need to update metrics: lru.RemoveOldest calls onEvicted()
	c.lru.RemoveOldest()
}

// Keys returns a slice of the keys in the cache, from oldest to newest.
func (c *Cache) Keys() []interface{} {
	if c == nil {
		return make([]interface{}, 0)
	}
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.lru.Keys()
}

// Len returns the number of items in the cache.
func (c *Cache) Len() int {
	if c == nil {
		return 0
	}
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.lru.Len()
}

// Metrics returns a copy of the cache's runtime properties.
func (c *Cache) Metrics() Metrics {
	if c == nil {
		return Metrics{}
	}
	return c.metrics
}

// CollectMetrics returns a copy of the cache's runtime properties.
func (c *Cache) CollectMetrics() jsonutil.GenericMarshaler {
	m := c.Metrics()
	return &m
}

// onEvict is the evict handler registered with the simple LRU and is used to
// update the item count. Relays to the external evict handler if present.
func (c *Cache) onEvict(key interface{}, value interface{}) {
	c.metrics.Remove()
	if c.evictHandler != nil {
		c.evictHandler(key, value)
	}
}

// getOrEvict gets the value for the given key using the regular write lock if
// requested. It handles a possible eviction with the optional evict function,
// but doesn't call the registered onEvict handler (for backwards
// compatibility). It also updates all necessary metrics accordingly.
// optFn is an optional function that gets called (within the write lock if
// requested).
func (c *Cache) getOrEvict(
	key interface{},
	lock bool,
	evict func(val interface{}) bool,
	optFn func()) (interface{}, bool) {

	if lock {
		// need the write lock, since this updates the recently-used list in
		// simple.LRU!
		c.lock.Lock()
		defer c.lock.Unlock()
	}

	if optFn != nil {
		defer optFn()
	}

	val, ok := c.lru.Get(key)
	if ok {
		if evict == nil || !evict(val) {
			c.metrics.Hit()
			return val, true
		}
		// item got evicted by custom optional evict function
		c.metrics.Remove()
	}
	c.metrics.Miss()

	return nil, false
}
