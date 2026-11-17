package syncutil

// Unlocker is the interface to unlock a locked mutex.
type Unlocker interface {
	Unlock()
}

// NamedLocks provides locks identified by a name of any comparable type. It is KeyedLocks with keys of type any, so
// a name is boxed into an interface when it is locked. Use KeyedLocks with the actual key type to avoid that.
//
// The zero value for NamedLocks is ready to be used.
//
// Usage:
//
//	nl := NamedLocks{}
//
//	l := nl.Lock("blub")
//	defer l.Unlock()
//	...
type NamedLocks = KeyedLocks[any]
