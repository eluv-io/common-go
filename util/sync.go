package util

// NoopLocker is a locker that does nothing.
type NoopLocker struct{}

func (n NoopLocker) Lock()   {}
func (n NoopLocker) Unlock() {}
