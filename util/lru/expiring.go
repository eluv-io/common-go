package lru

import "time"

// NewExpiringCache creates a new ExpiringCache.
func NewExpiringCache(maxSize int, maxAge time.Duration) *ExpiringCache {
	return &ExpiringCache{
		cache:  New(maxSize),
		maxAge: maxAge,
	}
}

// ExpiringCache is an LRU cache that evicts entries from the cache if they are
// older than the configured max age.
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
				ts:  time.Now(),
			}, nil
		},
		func(val interface{}) bool {
			if time.Now().Sub(val.(*expiringEntry).ts) > c.maxAge {
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
	ts  time.Time
}
