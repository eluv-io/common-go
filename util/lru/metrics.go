package lru

import (
	"encoding/json"
	"sync/atomic"

	"github.com/eluv-io/common-go/format/duration"
)

// Metrics collects runtime metrics of the LRU cache.
// NOTE: none of the fields are "atomic" or otherwise protected for concurrent
//
//	access, because the cache updating this information should already be
//	properly synchronized.
type Metrics struct {
	Name      string        // name of the cache
	Config    config        // static configuration
	Hits      *atomic.Int64 // Number of cache hits
	StaleHits *atomic.Int64 // Number of stale entries returned (expiring cache only)
	Misses    *atomic.Int64 // Number of cache misses
	Errors    *atomic.Int64 // Number of errors when trying to load/create a cache entry
	Added     *atomic.Int64 // Added - Removed = Current Size
	Removed   *atomic.Int64 // see Added
}

func MakeMetrics() Metrics {
	return Metrics{
		Hits:      &atomic.Int64{},
		Misses:    &atomic.Int64{},
		StaleHits: &atomic.Int64{},
		Errors:    &atomic.Int64{},
		Added:     &atomic.Int64{},
		Removed:   &atomic.Int64{},
	}
}

// config is the static configuration of Metrics
type config struct {
	MaxItems int
	MaxAge   duration.Spec
	Mode     string
}

func (c *Metrics) Copy() Metrics {
	ret := MakeMetrics()
	ret.Name = c.Name
	ret.Config = config{
		MaxItems: c.Config.MaxItems,
		MaxAge:   c.Config.MaxAge,
		Mode:     c.Config.Mode,
	}
	ret.Hits.Store(c.Hits.Load())
	ret.Misses.Store(c.Misses.Load())
	ret.StaleHits.Store(c.StaleHits.Load())
	ret.Errors.Store(c.Errors.Load())
	ret.Added.Store(c.Added.Load())
	ret.Removed.Store(c.Removed.Load())

	return ret
}

// Hit increments the Hits count.
func (c *Metrics) Hit() {
	c.Hits.Add(1)
}

// Miss increments the Misses count.
func (c *Metrics) Miss() {
	c.Misses.Add(1)
}

// StaleHit marks a hit as stale by incrementing the StaleHits count and decrementing the Hits count.
func (c *Metrics) StaleHit() {
	c.StaleHits.Add(1)
	c.Hits.Add(-1)
}

// UnMiss decrements the Misses count.
func (c *Metrics) UnMiss() {
	c.Misses.Add(-1)
}

// Error increments the Errors count.
func (c *Metrics) Error() {
	c.Errors.Add(1)
}

// Add increments the Added count.
func (c *Metrics) Add() {
	c.Added.Add(1)
}

// Remove increments the Removed count.
func (c *Metrics) Remove() {
	c.Removed.Add(1)
}

func (c *Metrics) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.MarshalGeneric())
}

func (c *Metrics) String() string {
	res, _ := json.Marshal(c.MarshalGeneric())
	return string(res)
}

func (c *Metrics) MarshalGeneric() interface{} {
	conf := map[string]interface{}{
		"max_items": c.Config.MaxItems,
	}
	m := map[string]interface{}{
		"config":     conf,
		"hits":       c.Hits.Load(),
		"stale_hits": c.StaleHits.Load(),
		"misses":     c.Misses.Load(),
		"errors":     c.Errors.Load(),
		"added":      c.Added.Load(),
		"removed":    c.Removed.Load(),
	}
	if c.Config.Mode != "" {
		conf["mode"] = c.Config.Mode
	}
	if c.Config.MaxAge != 0 {
		m["max_age"] = c.Config.MaxAge
	}
	return m
}
