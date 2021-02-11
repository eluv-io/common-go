package lru

import (
	"time"

	"github.com/qluvio/content-fabric/format/duration"
	"github.com/qluvio/content-fabric/format/utc"
	"github.com/qluvio/content-fabric/util/jsonutil"
)

// NewExpiringCache creates a new ExpiringCache.
func NewExpiringCache(maxSize int, maxAge duration.Spec) *ExpiringCache {
	res := &ExpiringCache{
		cache:  New(maxSize),
		maxAge: maxAge.Duration(),
	}
	res.cache.WithMaxAge(maxAge)
	return res
}

// ExpiringCache is an LRU cache that evicts entries from the cache when they
// reach the configured max age. Expired entries are evicted lazily, i.e. only
// when requested, and hence not garbage collected otherwise.
type ExpiringCache struct {
	cache  *Cache
	maxAge time.Duration
}

func (c *ExpiringCache) WithMode(mode ConstructionMode) *ExpiringCache {
	c.cache.WithMode(mode)
	return c
}

// GetOrCreate looks up a key's value from the cache, creating it if necessary.
//
//  - If the key does not exist, the given constructor function is called to
//    create a new value, store it at the key and return it. If the constructor
//    fails, no value is added to the cache and the error is returned.
//    Otherwise, the new value is added to the cache, and a boolean to mark any
//    evictions from the cache is returned as defined in the Add() method.
//  - If the key exists but is expired according to the max age, the current
//    value is discarded and re-created with the constructor function.
//  - If evict functions are passed and a non-expired cache entry exists, then
//    the first evict function is invoked with the cached value. If it returns
//    true, the value is discarded from the cache and the constructor is called.
func (c *ExpiringCache) GetOrCreate(
	key interface{},
	constructor func() (interface{}, error),
	evict ...func(val interface{}) bool) (val interface{}, evicted bool, err error) {

	val, evicted, err = c.cache.GetOrCreate(
		key,
		func() (interface{}, error) {
			val, err := constructor()
			if err != nil {
				return nil, err
			}
			return &expiringEntry{
				val: val,
				ts:  utc.Now(),
			}, nil
		},
		func(val interface{}) bool {
			if utc.Now().Sub(val.(*expiringEntry).ts) >= c.maxAge {
				return true
			}
			if len(evict) > 0 {
				return evict[0](val)
			}
			return false
		},
	)
	if err != nil {
		return nil, evicted, err
	}
	return val.(*expiringEntry).val, evicted, nil
}

func (c *ExpiringCache) Get(key interface{}) (interface{}, bool) {
	return c.cache.Get(key)
}

func (c *ExpiringCache) Remove(key interface{}) {
	c.cache.Remove(key)
}

// Metrics returns a copy of the cache's runtime properties.
func (c *ExpiringCache) Metrics() Metrics {
	return c.cache.Metrics()
}

// CollectMetrics returns a copy of the cache's runtime properties.
func (c *ExpiringCache) CollectMetrics() jsonutil.GenericMarshaler {
	m := c.Metrics()
	return &m
}

type expiringEntry struct {
	val interface{}
	ts  utc.UTC
}
