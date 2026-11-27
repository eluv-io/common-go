package lru

import (
	"io"
	"math"
	"sync"

	"github.com/maypok86/otter/v2"

	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/stringutil"
	"github.com/eluv-io/common-go/util/traceutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
)

// NewRefCache creates a new untyped RefCache with the given max size and optional callback.
func NewRefCache(
	maxSize int,
	callback Callback) *RefCache {

	return NewTypedRefCache[string, io.Closer](maxSize, callback)
}

// NewTypedRefCache creates a new TypedRefCache with the given max size and optional callback.
func NewTypedRefCache[K comparable, V io.Closer](
	maxSize int,
	callback Callback) *TypedRefCache[K, V] {

	if callback == nil {
		callback = NoopCallback{}
	}

	c := &TypedRefCache[K, V]{
		callback: callback,
		active:   make(map[K]*entry[V]),
		metrics:  MakeMetrics(),
	}
	if maxSize > 0 {
		c.lru = newSimpleLRU[K, *entry[V]](maxSize, c.closeEntry)
	}
	c.metrics.Config.MaxItems = maxSize
	return c
}

// NewWeightedRefCache creates a TypedRefCache whose idle resources are bounded by their total weight instead of their
// number. The weigh function returns the weight of a resource, e.g. its approximate size in bytes. A resource is
// weighed when its last reference is released, and again with Reweigh.
//
// The idle resources are kept in a W-TinyLFU cache (github.com/maypok86/otter), which admits a released resource only
// if it is used more frequently than the resources it would displace. A scan over many resources that are used once
// therefore does not evict the frequently used ones. A resource heavier than maxWeight is closed when it is released.
//
// Evictions happen asynchronously. Evicted resources are closed during the next call to the cache.
func NewWeightedRefCache[K comparable, V io.Closer](
	maxWeight uint64,
	weigh func(V) uint64,
	callback Callback) (*TypedRefCache[K, V], error) {

	return newWeightedRefCache[K, V](maxWeight, weigh, callback, nil)
}

// newWeightedRefCache creates a weighted TypedRefCache. A nil executor runs otter's maintenance on goroutines of its
// own. Tests pass an executor that runs it right away.
func newWeightedRefCache[K comparable, V io.Closer](
	maxWeight uint64,
	weigh func(V) uint64,
	callback Callback,
	executor func(fn func())) (*TypedRefCache[K, V], error) {

	e := errors.Template("NewWeightedRefCache", errors.K.Invalid)
	if maxWeight == 0 {
		return nil, e("reason", "max weight is 0")
	}
	if weigh == nil {
		return nil, e("reason", "weigh function is nil")
	}
	if callback == nil {
		callback = NoopCallback{}
	}

	c := &TypedRefCache[K, V]{
		callback: callback,
		active:   make(map[K]*entry[V]),
		metrics:  MakeMetrics(),
	}
	// a weight of 0 would exempt the entry from eviction
	c.weigh = func(res V) uint32 { return uint32(max(1, min(weigh(res), math.MaxUint32))) }
	var err error
	c.weighted, err = otter.New(&otter.Options[K, *entry[V]]{
		MaximumWeight: maxWeight,
		Weigher: func(_ K, ent *entry[V]) uint32 {
			// called by Set, which holds c.mutex
			ent.weight = c.weigh(ent.resource)
			return ent.weight
		},
		OnAtomicDeletion: c.onWeightedDeletion,
		Executor:         executor,
	})
	if err != nil {
		return nil, e(err)
	}
	c.metrics.Config.MaxWeight = maxWeight
	return c, nil
}

// Constructor is a function to create values.
type Constructor[V io.Closer] func() (V, error)

type Callback interface {
	OnOpen()    // called when a new resource is created (and added to the cache)
	OnClose()   // called when a resource is closed (evicted from the cache)
	OnGet()     // called when a resource is retrieved from the cache (also called on creation of resource)
	OnRelease() // called when a resource is release
}

// RefCache is an untyped version of TypedRefCache.
type RefCache = TypedRefCache[string, io.Closer]

// TypedRefCache implements a two-tier cache for shared resources that implement an
// explicit open/close semantic.
//
// A client opens (or creates) a resource with a call to GetOrCreate(). The
// client then uses the resource for a period of time and finally releases it
// with a call to Release(). Multiple clients (running on different goroutines)
// may request and use the same resource concurrently. As long as the resource
// is actively used by at least one client, it remains in the first tier of the
// cache, implemented with a map ("active").
//
// When the resource is released by the last client, it is moved into the second
// tier: an LRU cache of limited size. If it is requested again, it is moved
// back to tier 1.
//
// The resource is only closed once it gets evicted from the LRU cache (i.e. due
// to size restrictions).
//
// A cache created with NewWeightedRefCache keeps the second tier in a W-TinyLFU
// cache bounded by the weight of its resources instead.
//
// Resources are created through a constructor function passed to GetOrCreate()
// and closed with a call to their Close() function (resources must implement
// the io.Closer interface).
type TypedRefCache[K comparable, V io.Closer] struct {
	active   map[K]*entry[V]            // all active entries
	lru      *simpleLRU[K, *entry[V]]   // the LRU cache for inactive entries
	weighted *otter.Cache[K, *entry[V]] // the cache for inactive entries of a weighted cache, instead of lru
	weigh    func(V) uint32             // the weigher of a weighted cache
	mutex    sync.Mutex                 // mutex for access to active map
	callback Callback
	metrics  Metrics

	// evicted holds the entries evicted from weighted, to be closed. Otter reports evictions from its maintenance,
	// which may run while mutex is held, so they are closed later, holding mutex.
	evictedMutex sync.Mutex
	evicted      []evicted[K, V]
}

type entry[V io.Closer] struct {
	mutex    sync.Mutex
	resource V
	refCount int
	error    error  // a creation error
	closed   bool   // true once the resource is closed, guarded by the cache's mutex
	weight   uint32 // the weight in a weighted cache when it was last added, guarded by the cache's mutex
}

// evicted is an entry evicted from the weighted cache.
type evicted[K comparable, V io.Closer] struct {
	key K
	ent *entry[V]
}

func (e *entry[V]) validate() (resource V, err error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.resource, e.error
}

func (c *TypedRefCache[K, V]) WithName(name string) {
	c.metrics.Name = name
}

func (c *TypedRefCache[K, V]) GetOrCreate(key K, constructor Constructor[V]) (V, error) {
	var ent *entry[V]
	var exists bool
	var err error

	span := traceutil.StartSpan("lru.TypedRefCache.GetOrCreate")
	defer span.End()
	if span.IsRecording() {
		orgConstructor := constructor
		constructor = func() (V, error) {
			span := traceutil.StartSpan("constructor")
			defer span.End()
			return orgConstructor()
		}
		if c != nil {
			span.Attribute("cache", c.metrics.Name)
			span.Attribute("key", stringutil.ToString(key))
		}
	}

	// first check whether it's an active resource
	c.mutex.Lock()
	{
		c.closeEvicted()
		ent, exists = c.active[key]
		if exists {
			// it's an active resource
			ent.refCount++
		} else {
			if c.lru != nil {
				ent, exists = c.lru.Get(key)
			} else if c.weighted != nil {
				// The entry stays in the weighted cache while active, like in the LRU. GetIfPresent may still return an
				// entry that was evicted and closed, until otter removes it from its map.
				ent, exists = c.weighted.GetIfPresent(key)
				exists = exists && !ent.closed
			}
			if exists {
				// it's a cached resource in the LRU
				ent.refCount = 1
				// add it back to the active map
				c.active[key] = ent
			} else {
				// it doesn't exist - create the wrapper entry, lock it and add
				// it to the active map. The actual resource is created below.
				ent = &entry[V]{refCount: 1}
				ent.mutex.Lock()
				c.active[key] = ent
				c.metrics.Add()
				c.metrics.Miss()
			}
		}
	}
	c.mutex.Unlock()

	if exists {
		// the entry could still be created by another goroutine - since
		// validate() locks the mutex, this waits until the creation has
		// finished. If there is an error in the (concurrent) creation, we
		// return that error as well.
		res, err := ent.validate()
		c.mutex.Lock()
		if err != nil {
			c.metrics.Error()
		} else {
			c.metrics.Hit()
			c.callback.OnGet()
		}
		c.mutex.Unlock()
		return res, err
	}

	// the wrapper entry now exists, but not the resource... create it
	// we still have the lock on the entry
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = errors.E("TypedRefCache entry constructor", errors.K.Internal, "panic", r)
			}
		}()
		ent.resource, err = constructor()
	}()

	if err != nil {
		// mark the entry as failed
		ent.error = err
		ent.mutex.Unlock()

		// and remove it again from the active map
		c.mutex.Lock()
		delete(c.active, key)
		c.metrics.Error()
		c.mutex.Unlock()

		var zero V
		return zero, err
	}

	ent.mutex.Unlock()

	c.callback.OnOpen()
	c.callback.OnGet()
	return ent.resource, nil
}

// Release releases the given resource. Returns true if an eviction from the
// cache occurred (usually another entry than the one being released...). A
// weighted cache evicts asynchronously and always returns false.
func (c *TypedRefCache[K, V]) Release(key K) (evicted bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	defer c.closeEvicted()

	ent, found := c.active[key]
	if !found {
		// a cache usage error
		log.Warn("TypedRefCache: release called for unknown resource!",
			"key", key,
			"stack", errors.E("RefCache.Release"))
		return
	}

	_, err := ent.validate()
	if err != nil {
		// the entry is in the process of being created... that's similar to the
		// error above
		log.Warn("TypedRefCache: release called for resource under construction!",
			"key", key,
			"stack", errors.E("RefCache.Release"))
		return
	}

	c.callback.OnRelease()
	ent.refCount--
	if ent.refCount > 0 {
		// still in use
		return
	}

	if ent.refCount < 0 {
		// that should never happen and would indicate a bug in the cache implementation
		log.Error("TypedRefCache: reference count negative!",
			"key", key,
			"ref_count", ent.refCount,
			"stack", errors.E("RefCache.Release"))
		ent.refCount = 0
	}

	// remove from the active map
	// the resource is closed on LRU cache eviction only!
	delete(c.active, key)

	if c.lru != nil {
		evicted = c.lru.Add(key, ent)
	} else if c.weighted != nil {
		// An entry stays in the weighted cache while active. It is added again only if it was evicted meanwhile or its
		// weight changed. The hit in GetOrCreate already counted as a use.
		if e, found := c.weighted.GetEntryQuietly(key); !found || e.Value != ent || c.weigh(ent.resource) != ent.weight {
			c.weighted.Set(key, ent)
		}
	} else {
		c.closeEntry(key, ent)
	}

	return evicted
}

// Reweigh weighs the resource of the given key again, if the key is cached. Use it when the weight of a resource
// changed after its release, which may evict other resources. It does nothing in a cache that is not weighted.
func (c *TypedRefCache[K, V]) Reweigh(key K) {
	if c.weighted == nil {
		return
	}
	c.mutex.Lock()
	defer c.mutex.Unlock()
	defer c.closeEvicted()

	if e, found := c.weighted.GetEntryQuietly(key); found && c.weigh(e.Value.resource) != e.Value.weight {
		c.weighted.Set(key, e.Value)
	}
}

func (c *TypedRefCache[K, V]) Purge() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for key, ent := range c.active {
		delete(c.active, key)
		c.closeEntry(key, ent)
	}
	if c.lru != nil {
		c.lru.Purge()
	}
	if c.weighted != nil {
		var idle []evicted[K, V]
		for key, ent := range c.weighted.All() {
			idle = append(idle, evicted[K, V]{key: key, ent: ent})
		}
		c.weighted.InvalidateAll()
		for _, ev := range idle {
			c.closeEntry(ev.key, ev.ent)
		}
		c.closeEvicted()
	}
}

// Metrics returns a copy of the cache's runtime properties.
func (c *TypedRefCache[K, V]) Metrics() Metrics {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.weighted != nil {
		c.closeEvicted()
		c.metrics.Weight.Store(int64(c.weighted.WeightedSize()))
	}
	return c.metrics.Copy()
}

// CollectMetrics returns a copy of the cache's runtime properties.
func (c *TypedRefCache[K, V]) CollectMetrics() jsonutil.GenericMarshaler {
	m := c.Metrics()
	return &m
}

func (c *TypedRefCache[K, V]) closeEntry(key K, ent *entry[V]) {
	// close entry is always called holding c.mutex!
	if ent.closed {
		return
	}
	res, err := ent.validate()
	if err != nil {
		log.Warn("TypedRefCache: closeEntry called for invalid resource!", "key", key)
		return
	}
	// the resource is only removed from the cache if it's gone from the lru AND the active map. A different entry
	// under the same key was created after this one was evicted, and does not keep this one open.
	if active, found := c.active[key]; found && active == ent {
		return
	}
	if c.weighted != nil {
		// an entry evicted while active is added again on its release, and must not be closed then
		if e, found := c.weighted.GetEntryQuietly(key); found && e.Value == ent {
			return
		}
	}
	ent.closed = true
	_ = res.Close()
	c.metrics.Remove()
	c.callback.OnClose()
}

// onWeightedDeletion is called by the weighted cache when it deletes an entry. It collects the entries evicted for
// their weight. It must not take c.mutex: otter may call it while a call to the cache holds the mutex.
func (c *TypedRefCache[K, V]) onWeightedDeletion(e otter.DeletionEvent[K, *entry[V]]) {
	if !e.WasEvicted() {
		// an invalidation by Purge, or a replacement by Release or Reweigh
		return
	}
	c.evictedMutex.Lock()
	c.evicted = append(c.evicted, evicted[K, V]{key: e.Key, ent: e.Value})
	c.evictedMutex.Unlock()
}

// closeEvicted closes the entries evicted from the weighted cache. Must be called holding c.mutex.
func (c *TypedRefCache[K, V]) closeEvicted() {
	if c.weighted == nil {
		return
	}
	c.evictedMutex.Lock()
	evicted := c.evicted
	c.evicted = nil
	c.evictedMutex.Unlock()
	for _, ev := range evicted {
		c.closeEntry(ev.key, ev.ent)
	}
}

// ---------------------------------------------------------------------------------------------------------------------

type NoopCallback struct{}

func (q NoopCallback) OnOpen()    {}
func (q NoopCallback) OnClose()   {}
func (q NoopCallback) OnGet()     {}
func (q NoopCallback) OnRelease() {}
