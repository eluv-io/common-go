package syncutil

import "sync"

// legacyNamedLocks is the NamedLocks implementation before KeyedLocks: one mutex and map for all names, a boxed
// name, and a lock and release closure allocated per lock. It is kept as the baseline of the benchmarks.
type legacyNamedLocks struct {
	mutex sync.Mutex
	named map[interface{}]*legacyLock
}

func (n *legacyNamedLocks) Lock(name interface{}) Unlocker {
	l := n.get(name)
	l.mutex.Lock()
	return l
}

func (n *legacyNamedLocks) get(name interface{}) *legacyLock {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	if n.named == nil {
		n.named = make(map[interface{}]*legacyLock)
	}

	l, found := n.named[name]
	if found {
		l.refCount++
		return l
	}
	l = &legacyLock{name: name}
	l.onUnlock = func() {
		n.release(l)
	}
	n.named[name] = l
	return l
}

func (n *legacyNamedLocks) release(l *legacyLock) {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	if l.refCount > 0 {
		l.refCount--
		return
	}

	delete(n.named, l.name)
}

type legacyLock struct {
	mutex    sync.Mutex
	refCount int

	name     interface{}
	onUnlock func()
}

func (l *legacyLock) Unlock() {
	l.onUnlock()
	l.mutex.Unlock()
}
