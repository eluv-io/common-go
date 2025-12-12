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
//
//		nl := NamedLocks{}
//
//		l := nl.Lock("blub")
//	 defer l.Unlock()
//	 ...
type NamedLocks struct {
	named sync.Map // interface{} -> lock
}

// Lock gets or creates the lock for the given name, calls its Lock() method and
// returns its Unlocker (i.e. the "unlocking half" of the sync.Locker
// interface). This ensures that the returned lock is not stored and re-used.
// Instead, simply call NamedLocks.Lock() again.
func (n *NamedLocks) Lock(name interface{}) Unlocker {
	return n.get(name, true)
}

func (n *NamedLocks) RLock(name interface{}) Unlocker {
	return n.get(name, false)
}

func (n *NamedLocks) get(name interface{}, write bool) *unlocker {
	l := &lock{name: name, refCount: 1}
	l.onUnlock = func() {
		n.release(l)
	}
	l.lock(write)
	for {
		v, found := n.named.LoadOrStore(name, l)
		if !found {
			break
		} else {
			l2 := v.(*lock)
			l2.lock(write)
			if l2.refCount > 0 {
				l.unlock(write)
				l2.refCount++
				l = l2
				break
			}
			l2.unlock(write)
		}
	}
	u := &unlocker{unlock: func() {
		l.onUnlock()
		l.unlock(write)
	}}
	return u
}

func (n *NamedLocks) release(l *lock) {
	l.refCount--
	if l.refCount == 0 {
		n.named.Delete(l.name)
	}
}

type lock struct {
	mutex    sync.RWMutex
	refCount int

	name     interface{}
	onUnlock func()
}

func (l *lock) lock(write bool) {
	if write {
		l.mutex.Lock()
	} else {
		l.mutex.RLock()
	}
}

func (l *lock) unlock(write bool) {
	if write {
		l.mutex.Unlock()
	} else {
		l.mutex.RUnlock()
	}
}

type unlocker struct {
	unlock func()
}

func (u *unlocker) Unlock() {
	if u.unlock != nil {
		u.unlock()
	}
}
