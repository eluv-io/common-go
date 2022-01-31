package lru

import (
	"io"
	"sync"

	"github.com/eluv-io/log-go"
	"github.com/hashicorp/golang-lru/simplelru"
	"go.opentelemetry.io/otel/api/kv"
	"go.opentelemetry.io/otel/api/trace"

	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/stringutil"
	"github.com/eluv-io/common-go/util/traceutil"
)

func NewRefCache(
	maxSize int,
	callback Callback) *RefCache {

	c := &RefCache{
		callback: callback,
		active:   make(map[string]*entry),
	}
	if maxSize > 0 {
		c.lru, _ = simplelru.NewLRU(maxSize, c.onEvict)
	}
	c.metrics.Config.MaxItems = maxSize
	return c
}

type Constructor func() (io.Closer, error)

type Callback interface {
	OnOpen()    // called when a new resource is created (and added to the cache)
	OnClose()   // called when a resource is closed (evicted from the cache)
	OnGet()     // called when a resource is retrieved from the cache (also called on creation of resource)
	OnRelease() // called when a resource is release
}

// RefCache implements a two-tier cache for shared resources that implement an
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
type RefCache struct {
	active   map[string]*entry // all active entries
	lru      *simplelru.LRU    // the LRU cache for inactive entries
	mutex    sync.Mutex        // mutex for access to active map
	callback Callback
	metrics  Metrics
}

type entry struct {
	mutex    sync.Mutex
	resource io.Closer
	refCount int
	error    error // a creation error
}

func (e *entry) validate() (resource io.Closer, err error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.resource, e.error
}

func (c *RefCache) WithName(name string) {
	c.metrics.Name = name
}

func (c *RefCache) GetOrCreate(key string, constructor Constructor) (interface{}, error) {
	var val interface{}
	var ent *entry
	var exists bool
	var err error

	span := traceutil.StartSpan(
		"lru.RefCache.GetOrCreate",
		func(sc *trace.StartConfig) {
			// this is only called if tracing is enabled!
			orgConstructor := constructor
			constructor = func() (io.Closer, error) {
				span := traceutil.StartSpan("constructor")
				defer span.End()
				return orgConstructor()
			}
			if c != nil {
				sc.Attributes = append(sc.Attributes, kv.String("cache", c.metrics.Name))
				sc.Attributes = append(sc.Attributes, kv.String("key", stringutil.ToString(key)))
			}
		},
	)
	defer span.End()

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
				ent = val.(*entry)
				ent.refCount = 1
				// add it back to the active map
				c.active[key] = ent
			} else {
				// it doesn't exist - create the wrapper entry, lock it and add
				// it to the active map. The actual resource is created below.
				ent = &entry{refCount: 1}
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
		val, err = ent.validate()
		c.mutex.Lock()
		if err != nil {
			c.metrics.Error()
		} else {
			c.metrics.Hit()
			c.callback.OnGet()
		}
		c.mutex.Unlock()
		return val, err
	}

	// the wrapper entry now exists, but not the resource... create it
	// we still have the lock on the entry
	ent.resource, err = constructor()

	if err != nil {
		// mark the entry as failed
		ent.error = err
		ent.mutex.Unlock()

		// and remove it again from the active map
		c.mutex.Lock()
		delete(c.active, key)
		c.metrics.Error()
		c.mutex.Unlock()

		return nil, err
	}

	ent.mutex.Unlock()

	c.callback.OnOpen()
	c.callback.OnGet()
	return ent.resource, nil
}

// Release releases the given resource. Returns true if an eviction from the
// cache occurred (usually another entry than the one being released...)
func (c *RefCache) Release(key string) (evicted bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	ent, found := c.active[key]
	if !found {
		// a cache usage error
		log.Warn("RefCache: release called for unknown resource!", "key", key)
		return
	}

	_, err := ent.validate()
	if err != nil {
		// the entry is in the process of being created... that's similar to the
		// error above
		log.Warn("RefCache: release called for resource under construction!", "key", key)
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
		log.Error("RefCache: reference count negative!", "key", key, "ref_count", ent.refCount)
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

func (c *RefCache) Purge() {
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
func (c *RefCache) Metrics() Metrics {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.metrics
}

// CollectMetrics returns a copy of the cache's runtime properties.
func (c *RefCache) CollectMetrics() jsonutil.GenericMarshaler {
	m := c.Metrics()
	return &m
}

func (c *RefCache) onEvict(key interface{}, val interface{}) {
	c.closeEntry(key, val.(*entry))
}

func (c *RefCache) closeEntry(key interface{}, ent *entry) {
	// close entry is always called holding c.mutex!
	res, err := ent.validate()
	if err != nil {
		log.Warn("RefCache: closeEntry called for invalid resource!", "key", key)
		return
	}
	if _, found := c.active[key.(string)]; !found {
		// the resource is only removed from the cache if it's gone from the
		// lru AND the active map.
		_ = res.Close()
		c.metrics.Remove()
		c.callback.OnClose()
	}
}
