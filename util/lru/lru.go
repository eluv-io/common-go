// The lru package provides a simple LRU cache. It is a small adaption of the
// LRU implementation of github.com/hashicorp/golang-lru/lru.go that adds the
// function GetOrCreate() to atomically get a cached value or create it if it
// does not yet exists in the cache.
package lru

import (
	"sync"

	"github.com/hashicorp/golang-lru/simplelru"
)

// Cache is a thread-safe fixed size LRU cache.
type Cache struct {
	lru  *simplelru.LRU
	lock sync.RWMutex
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
		size = 1
	}
	lru, _ := simplelru.NewLRU(size, simplelru.EvictCallback(onEvicted))
	c := &Cache{
		lru: lru,
	}
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
	return c.lru.Add(key, value)
}

// Get looks up a key's value from the cache.
func (c *Cache) Get(key interface{}) (interface{}, bool) {
	if c == nil {
		return nil, false
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.lru.Get(key)
}

// GetOrCreate looks up a key's value from the cache, creating it if necessary.
// If the key does not exist, the given constructor function is called to
// create a new value, store it at the key and return it. If the constructor
// fails, no value is added to the cache and the error is returned. Otherwise,
// the new value is added to the cache, and a boolean to mark any evictions
// from the cache is returned as defined in the Add() method.
func (c *Cache) GetOrCreate(key interface{}, constructor func() (interface{}, error)) (val interface{}, evicted bool, err error) {
	if c == nil {
		val, err = constructor()
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()

	val, ok := c.lru.Get(key)
	if ok {
		return
	}

	val, err = constructor()
	if err != nil {
		return
	}
	evicted = c.lru.Add(key, val)
	return
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

// ContainsOrAdd checks if a key is in the cache  without updating the
// recent-ness or deleting it for being stale,  and if not, adds the value.
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
		evict := c.lru.Add(key, value)
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
	c.lru.Remove(key)
}

// RemoveOldest removes the oldest item from the cache.
func (c *Cache) RemoveOldest() {
	if c == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
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
