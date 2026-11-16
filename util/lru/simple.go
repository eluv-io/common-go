package lru

// simpleLRU is a fixed-size LRU of typed keys and values. It is not safe for concurrent use: TypedCache and
// TypedRefCache guard it with their own mutex.
//
// The entries are in a map and in a doubly linked list from the newest to the oldest. A removed or evicted entry is
// kept for reuse, so adding a key allocates nothing once the cache has been full. The free entries and the entries in
// use never exceed the size, since an entry is freed only after it was in use.
type simpleLRU[K comparable, V any] struct {
	size  int
	items map[K]*simpleEntry[K, V]
	// root is the sentinel of the list. root.next is the newest entry and root.prev the oldest.
	root simpleEntry[K, V]
	// free is a list of removed entries for reuse, linked through their next field.
	free    *simpleEntry[K, V]
	onEvict func(key K, value V)
}

type simpleEntry[K comparable, V any] struct {
	key   K
	value V
	prev  *simpleEntry[K, V]
	next  *simpleEntry[K, V]
}

// newSimpleLRU creates an LRU that holds up to size entries. size must be positive. onEvict is called for each entry
// that is evicted, removed or purged, and may be nil.
func newSimpleLRU[K comparable, V any](size int, onEvict func(key K, value V)) *simpleLRU[K, V] {
	c := &simpleLRU[K, V]{
		size:    size,
		items:   make(map[K]*simpleEntry[K, V]),
		onEvict: onEvict,
	}
	c.root.next = &c.root
	c.root.prev = &c.root
	return c
}

// Add adds a value or replaces the value of an existing key, and makes the key the newest. Replacing a value is not an
// eviction. Returns true if adding the key evicted the oldest one.
func (c *simpleLRU[K, V]) Add(key K, value V) (evicted bool) {
	if e, ok := c.items[key]; ok {
		e.value = value
		c.moveToFront(e)
		return false
	}
	e := c.take()
	e.key = key
	e.value = value
	c.pushFront(e)
	c.items[key] = e
	if len(c.items) > c.size {
		c.removeEntry(c.root.prev)
		return true
	}
	return false
}

// Get returns the value of key and makes the key the newest.
func (c *simpleLRU[K, V]) Get(key K) (value V, ok bool) {
	e, ok := c.items[key]
	if !ok {
		return value, false
	}
	c.moveToFront(e)
	return e.value, true
}

// Peek returns the value of key without making it the newest.
func (c *simpleLRU[K, V]) Peek(key K) (value V, ok bool) {
	e, ok := c.items[key]
	if !ok {
		return value, false
	}
	return e.value, true
}

// Contains returns whether key is in the cache, without making it the newest.
func (c *simpleLRU[K, V]) Contains(key K) bool {
	_, ok := c.items[key]
	return ok
}

// Remove removes key and returns whether it was in the cache.
func (c *simpleLRU[K, V]) Remove(key K) bool {
	e, ok := c.items[key]
	if !ok {
		return false
	}
	c.removeEntry(e)
	return true
}

// RemoveOldest removes the oldest entry and returns it.
func (c *simpleLRU[K, V]) RemoveOldest() (key K, value V, ok bool) {
	if len(c.items) == 0 {
		return key, value, false
	}
	e := c.root.prev
	key, value = e.key, e.value
	c.removeEntry(e)
	return key, value, true
}

// GetOldest returns the oldest entry.
func (c *simpleLRU[K, V]) GetOldest() (key K, value V, ok bool) {
	if len(c.items) == 0 {
		return key, value, false
	}
	e := c.root.prev
	return e.key, e.value, true
}

// Keys returns the keys from the oldest to the newest.
func (c *simpleLRU[K, V]) Keys() []K {
	keys := make([]K, 0, len(c.items))
	for e := c.root.prev; e != &c.root; e = e.prev {
		keys = append(keys, e.key)
	}
	return keys
}

// Len returns the number of entries.
func (c *simpleLRU[K, V]) Len() int {
	return len(c.items)
}

// Purge removes all entries, from the oldest to the newest, and drops the entries kept for reuse.
func (c *simpleLRU[K, V]) Purge() {
	e := c.root.prev
	c.items = make(map[K]*simpleEntry[K, V])
	c.root.next = &c.root
	c.root.prev = &c.root
	c.free = nil
	if c.onEvict == nil {
		return
	}
	for ; e != &c.root; e = e.prev {
		c.onEvict(e.key, e.value)
	}
}

// removeEntry unlinks e, keeps it for reuse and calls the evict callback.
func (c *simpleLRU[K, V]) removeEntry(e *simpleEntry[K, V]) {
	e.prev.next = e.next
	e.next.prev = e.prev
	delete(c.items, e.key)
	key, value := e.key, e.value
	c.release(e)
	if c.onEvict != nil {
		c.onEvict(key, value)
	}
}

// take returns an entry for reuse, or a new one.
func (c *simpleLRU[K, V]) take() *simpleEntry[K, V] {
	e := c.free
	if e == nil {
		return &simpleEntry[K, V]{}
	}
	c.free = e.next
	e.next = nil
	return e
}

// release keeps e for reuse. It drops the key and value, so that the entry does not keep them alive.
func (c *simpleLRU[K, V]) release(e *simpleEntry[K, V]) {
	*e = simpleEntry[K, V]{next: c.free}
	c.free = e
}

func (c *simpleLRU[K, V]) moveToFront(e *simpleEntry[K, V]) {
	if c.root.next == e {
		return
	}
	e.prev.next = e.next
	e.next.prev = e.prev
	c.pushFront(e)
}

func (c *simpleLRU[K, V]) pushFront(e *simpleEntry[K, V]) {
	e.prev = &c.root
	e.next = c.root.next
	c.root.next.prev = e
	c.root.next = e
}
