package syncutil

import "sync"

// Unlocker is the interface to unlock a locked mutex.
type Unlocker interface {
	Unlock()
}

// NamedLocks provides locks identified by a name. The locks are created
// dynamically if they don't exist when requested, and are garbage collected
// when not in use anymore. Requesting a lock that is already in use will return
// the same lock instance, thereby guaranteeing mutual exclusion scoped to that
// name.
//
// The zero value for NamedLocks is ready to be used.
//
// Usage:
// 	nl := NamedLocks{}
//
//	l := nl.Lock("blub")
//  defer l.Unlock()
//  ...
type NamedLocks struct {
	mutex sync.Mutex
	named map[interface{}]*lock
}

// Lock gets or creates the lock for the given name, calls its Lock() method and
// returns its Unlocker (i.e. the "unlocking half" of the sync.Locker
// interface). This ensures that the returned lock is not stored and re-used.
// Instead simply call NamedLocks.Lock() again.
func (n *NamedLocks) Lock(name interface{}) Unlocker {
	l := n.get(name)
	l.mutex.Lock()
	return l
}

func (n *NamedLocks) get(name interface{}) *lock {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	if n.named == nil {
		n.named = make(map[interface{}]*lock)
	}

	l, found := n.named[name]
	if found {
		l.refCount++
		return l
	}
	l = &lock{name: name}
	l.onUnlock = func() {
		n.release(l)
	}
	n.named[name] = l
	return l
}

func (n *NamedLocks) release(l *lock) {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	if l.refCount > 0 {
		l.refCount--
		return
	}

	delete(n.named, l.name)
}

type lock struct {
	mutex    sync.Mutex
	refCount int

	name     interface{}
	onUnlock func()
}

func (l *lock) Unlock() {
	l.onUnlock()
	l.mutex.Unlock()
}
