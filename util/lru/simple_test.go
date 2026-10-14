package lru

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// evictRecorder records the calls of a simpleLRU's evict callback.
type evictRecorder struct {
	keys   []string
	values []int
}

func (r *evictRecorder) onEvict(key string, value int) {
	r.keys = append(r.keys, key)
	r.values = append(r.values, value)
}

func newRecordingLRU(size int) (*simpleLRU[string, int], *evictRecorder) {
	r := &evictRecorder{}
	return newSimpleLRU[string, int](size, r.onEvict), r
}

func TestSimpleLRUAddGet(t *testing.T) {
	c, r := newRecordingLRU(3)
	require.False(t, c.Add("a", 1))
	require.False(t, c.Add("b", 2))
	require.False(t, c.Add("c", 3))
	require.Equal(t, 3, c.Len())

	for key, want := range map[string]int{"a": 1, "b": 2, "c": 3} {
		got, ok := c.Get(key)
		require.True(t, ok, key)
		require.Equal(t, want, got, key)
		got, ok = c.Peek(key)
		require.True(t, ok, key)
		require.Equal(t, want, got, key)
		require.True(t, c.Contains(key), key)
	}

	_, ok := c.Get("x")
	require.False(t, ok)
	_, ok = c.Peek("x")
	require.False(t, ok)
	require.False(t, c.Contains("x"))
	require.Empty(t, r.keys)
}

// TestSimpleLRUEvictsLeastRecentlyUsed: a Get makes a key the newest, so the next Add at capacity evicts another one.
func TestSimpleLRUEvictsLeastRecentlyUsed(t *testing.T) {
	c, r := newRecordingLRU(2)
	c.Add("a", 1)
	c.Add("b", 2)
	c.Get("a")
	require.True(t, c.Add("c", 3))
	require.Equal(t, []string{"b"}, r.keys)
	require.Equal(t, []int{2}, r.values)
	require.Equal(t, []string{"a", "c"}, c.Keys())
}

// TestSimpleLRUPeekKeepsOrder: Peek and Contains do not make a key the newest.
func TestSimpleLRUPeekKeepsOrder(t *testing.T) {
	c, r := newRecordingLRU(2)
	c.Add("a", 1)
	c.Add("b", 2)
	c.Peek("a")
	c.Contains("a")
	require.True(t, c.Add("c", 3))
	require.Equal(t, []string{"a"}, r.keys)
}

// TestSimpleLRUUpdate: adding an existing key replaces its value and makes it the newest, without an eviction.
func TestSimpleLRUUpdate(t *testing.T) {
	c, r := newRecordingLRU(2)
	c.Add("a", 1)
	c.Add("b", 2)
	require.False(t, c.Add("a", 10))
	require.Equal(t, 2, c.Len())
	got, _ := c.Get("a")
	require.Equal(t, 10, got)
	require.Equal(t, []string{"b", "a"}, c.Keys())
	require.Empty(t, r.keys)
}

func TestSimpleLRURemove(t *testing.T) {
	c, r := newRecordingLRU(3)
	c.Add("a", 1)
	c.Add("b", 2)
	require.True(t, c.Remove("a"))
	require.False(t, c.Remove("a"))
	require.Equal(t, []string{"a"}, r.keys)
	require.Equal(t, []int{1}, r.values)
	require.Equal(t, []string{"b"}, c.Keys())
}

func TestSimpleLRUOldest(t *testing.T) {
	c, r := newRecordingLRU(3)
	_, _, ok := c.GetOldest()
	require.False(t, ok)
	_, _, ok = c.RemoveOldest()
	require.False(t, ok)

	c.Add("a", 1)
	c.Add("b", 2)
	key, value, ok := c.GetOldest()
	require.True(t, ok)
	require.Equal(t, "a", key)
	require.Equal(t, 1, value)
	require.Equal(t, 2, c.Len())

	key, value, ok = c.RemoveOldest()
	require.True(t, ok)
	require.Equal(t, "a", key)
	require.Equal(t, 1, value)
	require.Equal(t, []string{"a"}, r.keys)
	require.Equal(t, []string{"b"}, c.Keys())
}

// TestSimpleLRUPurge: Purge evicts every entry, from oldest to newest, and the cache works afterwards.
func TestSimpleLRUPurge(t *testing.T) {
	c, r := newRecordingLRU(3)
	c.Add("a", 1)
	c.Add("b", 2)
	c.Add("c", 3)
	c.Purge()
	require.Equal(t, []string{"a", "b", "c"}, r.keys)
	require.Equal(t, []int{1, 2, 3}, r.values)
	require.Zero(t, c.Len())
	require.Empty(t, c.Keys())

	c.Add("d", 4)
	got, ok := c.Get("d")
	require.True(t, ok)
	require.Equal(t, 4, got)
}

// TestSimpleLRUAllocateNothing: once the cache has been full, adding a new key reuses the entry of an evicted or
// removed one.
func TestSimpleLRUAllocateNothing(t *testing.T) {
	const size = 4
	names := make([]string, 4*size)
	for i := range names {
		names[i] = fmt.Sprintf("k%d", i)
	}
	c := newSimpleLRU[string, int](size, nil)
	for _, name := range names {
		c.Add(name, 1)
	}

	t.Run("add and evict", func(t *testing.T) {
		i := 0
		require.Zero(t, testing.AllocsPerRun(100, func() {
			c.Add(names[i%len(names)], 1)
			i++
		}))
	})
	t.Run("remove and add", func(t *testing.T) {
		require.Zero(t, testing.AllocsPerRun(100, func() {
			key, _, _ := c.GetOldest()
			c.Remove(key)
			c.Add(key, 1)
		}))
	})
}

// TestSimpleLRUReleasesValues: a removed entry kept for reuse no longer references its key and value.
func TestSimpleLRUReleasesValues(t *testing.T) {
	c := newSimpleLRU[string, *int](2, nil)
	value := 1
	c.Add("a", &value)
	c.Remove("a")
	require.NotNil(t, c.free)
	require.Empty(t, c.free.key)
	require.Nil(t, c.free.value)
}
