package lru

import (
	"io"
	"sync"

	"github.com/hashicorp/golang-lru/simplelru"

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
		c.lru, _ = simplelru.NewLRU(maxSize, c.onEvict)
	}
	c.metrics.Config.MaxItems = maxSize
	return c
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
// Resources are created through a constructor function passed to GetOrCreate()
// and closed with a call to their Close() function (resources must implement
// the io.Closer interface).
type TypedRefCache[K comparable, V io.Closer] struct {
	active   map[K]*entry[V] // all active entries
	lru      *simplelru.LRU  // the LRU cache for inactive entries
	mutex    sync.Mutex      // mutex for access to active map
	callback Callback
	metrics  Metrics
}

type entry[V io.Closer] struct {
	mutex    sync.Mutex
	resource V
	refCount int
	error    error // a creation error
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
	var val interface{}
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
		ent, exists = c.active[key]
		if exists {
			// it's an active resource
			ent.refCount++
		} else {
			if c.lru != nil {
				val, exists = c.lru.Get(key)
			}
			if exists {
				// it's a cached resource in the LRU
				ent = val.(*entry[V])
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
// cache occurred (usually another entry than the one being released...)
func (c *TypedRefCache[K, V]) Release(key K) (evicted bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

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
	} else {
		c.closeEntry(key, ent)
	}

	return evicted
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
}

// Metrics returns a copy of the cache's runtime properties.
func (c *TypedRefCache[K, V]) Metrics() Metrics {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.metrics.Copy()
}

// CollectMetrics returns a copy of the cache's runtime properties.
func (c *TypedRefCache[K, V]) CollectMetrics() jsonutil.GenericMarshaler {
	m := c.Metrics()
	return &m
}

func (c *TypedRefCache[K, V]) onEvict(key interface{}, val interface{}) {
	c.closeEntry(key.(K), val.(*entry[V]))
}

func (c *TypedRefCache[K, V]) closeEntry(key K, ent *entry[V]) {
	// close entry is always called holding c.mutex!
	res, err := ent.validate()
	if err != nil {
		log.Warn("TypedRefCache: closeEntry called for invalid resource!", "key", key)
		return
	}
	if _, found := c.active[key]; !found {
		// the resource is only removed from the cache if it's gone from the
		// lru AND the active map.
		_ = res.Close()
		c.metrics.Remove()
		c.callback.OnClose()
	}
}

// ---------------------------------------------------------------------------------------------------------------------

type NoopCallback struct{}

func (q NoopCallback) OnOpen()    {}
func (q NoopCallback) OnClose()   {}
func (q NoopCallback) OnGet()     {}
func (q NoopCallback) OnRelease() {}
