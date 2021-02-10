package lru

import (
	"encoding/json"

	"github.com/qluvio/content-fabric/format/duration"
)

// Metrics collects runtime metrics of the LRU cache.
// NOTE: none of the fields are "atomic" or otherwise protected for concurrent
//       access, because the cache updating this information should already be
//       properly synchronized.
type Metrics struct {
	Name   string   // name of the cache
	Config struct { // static configuration
		MaxItems int
		MaxAge   duration.Spec
		Mode     string
	}
	Hits         int64 // Number of cache hits
	Misses       int64 // Number of cache misses
	Errors       int64 // Number of errors when trying to load/create a cache entry
	ItemsAdded   int64 // ItemsAdded - ItemsRemoved = Current Size
	ItemsRemoved int64 // see ItemsAdded
}

// Hit increments the Hits count.
func (c *Metrics) Hit() {
	c.Hits++
}

// Miss increments the Misses count.
func (c *Metrics) Miss() {
	c.Misses++
}

// UnMiss decrements the Misses count.
func (c *Metrics) UnMiss() {
	c.Misses--
}

// Error increments the Errors count.
func (c *Metrics) Error() {
	c.Errors++
}

// ItemAdded increments the ItemsAdded count.
func (c *Metrics) ItemAdded() {
	c.ItemsAdded++
}

// ItemRemoved increments the ItemsRemoved count.
func (c *Metrics) ItemRemoved() {
	c.ItemsRemoved++
}

func (c *Metrics) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.MarshalGeneric())
}

func (c *Metrics) String() string {
	res, _ := json.Marshal(c.MarshalGeneric())
	return string(res)
}

func (c *Metrics) MarshalGeneric() interface{} {
	m := map[string]interface{}{
		"config": map[string]interface{}{
			"max_items": c.Config.MaxItems,
			"mode":      c.Config.Mode,
		},
		"hits":          c.Hits,
		"misses":        c.Misses,
		"errors":        c.Errors,
		"items_added":   c.ItemsAdded,
		"items_removed": c.ItemsRemoved,
	}
	if c.Config.MaxAge != 0 {
		m["max_age"] = c.Config.MaxAge
	}
	return m
}
