package lru

import (
	"time"

	"github.com/qluvio/content-fabric/format/utc"
)

// NewExpiringCache creates a new ExpiringCache.
func NewExpiringCache(maxSize int, maxAge time.Duration) *ExpiringCache {
	return &ExpiringCache{
		cache:  New(maxSize),
		maxAge: maxAge,
	}
}

// ExpiringCache is an LRU cache that evicts entries from the cache when they
// reach the configured max age. Expired entries are evicted lazily, i.e. only
// when requested, and hence not garbage collected otherwise.
type ExpiringCache struct {
	cache  *Cache
	maxAge time.Duration
}

// GetOrCreate gets the cached entry or creates a new one if it doesn't exist.
// See lru.Cache.GetOrCreate() for details.
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

type expiringEntry struct {
	val interface{}
	ts  utc.UTC
}
