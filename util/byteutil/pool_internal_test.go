package byteutil

import "sync"

// SetLocker allows to set the internal locker in tests
func (p *Pool) SetLocker(l sync.Locker) {
	p.locker = l
}
