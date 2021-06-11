package sessiontracker

import (
	"encoding/json"
	"sync"

	"github.com/qluvio/content-fabric/format/duration"
	"github.com/qluvio/content-fabric/format/utc"
	"github.com/qluvio/content-fabric/util/lru"
)

// Tracker is the interface for a generic session tracker. It manages a list of
// observed session IDs and removes them automatically upon expiration.
type Tracker interface {
	Update(sessionID string)
	Count() int
	List() []SessionInfo
	Purge()
	SessionMetrics() SessionMetrics
	Metrics() lru.Metrics
}

type SessionInfo struct {
	ID          string
	LastUpdated utc.UTC
}

func New(maxAge duration.Spec) Tracker {
	t := &tracker{
		sessions: lru.NewExpiringCache(1_000_000, maxAge),
	}
	t.sessions.WithEvictHandler(t.onEvicted)
	return t
}

////////////////////////////////////////////////////////////////////////////////

type SessionMetrics struct {
	Added   int64 // sessions added (Added - Removed = Current Size)
	Removed int64 // sessions removed
}

func (c *SessionMetrics) String() string {
	res, _ := json.Marshal(c.MarshalGeneric())
	return string(res)
}

func (c *SessionMetrics) MarshalGeneric() interface{} {
	m := map[string]interface{}{
		"added":   c.Added,
		"removed": c.Removed,
	}
	return m
}

////////////////////////////////////////////////////////////////////////////////

type tracker struct {
	mutex    sync.Mutex
	sessions *lru.ExpiringCache
	metrics  SessionMetrics
}

func (t *tracker) Metrics() lru.Metrics {
	return t.sessions.Metrics()
}

func (t *tracker) SessionMetrics() SessionMetrics {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.sessions.EvictExpired()
	return t.metrics
}

func (t *tracker) Update(sessionID string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.sessions.EvictExpired()
	isNew, _ := t.sessions.Update(sessionID, sessionID)
	if isNew {
		t.metrics.Added++
	}
}

func (t *tracker) Count() int {
	return t.sessions.Len()
}

func (t *tracker) List() []SessionInfo {
	entries := t.sessions.Entries()
	res := make([]SessionInfo, len(entries))
	for i, entry := range entries {
		res[i] = SessionInfo{
			ID:          entry.Value().(string),
			LastUpdated: entry.LastUpdated(),
		}
	}
	return res
}

func (t *tracker) Purge() {
	t.sessions.Purge()
}

func (t *tracker) onEvicted(key interface{}, value interface{}) {
	// called from Update or SessionMetrics while holding mutex
	t.metrics.Removed++
}
