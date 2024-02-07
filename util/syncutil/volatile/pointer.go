package volatile

import (
	"encoding/json"
	"sync/atomic"

	"github.com/eluv-io/common-go/util/syncutil/atomicutil"
)

type Pointer[T any] struct {
	*atomic.Pointer[T]
}

func NewPointer[T any](val *T) *Pointer[T] {
	return &Pointer[T]{Pointer: atomicutil.Pointer[T](val)}
}

func (p *Pointer[T]) MarshalJSON() ([]byte, error) {
	val := p.Load()
	return json.Marshal(&val)
}

func (p *Pointer[T]) UnmarshalJSON(b []byte) error {
	var val T
	err := json.Unmarshal(b, &val)
	if err != nil {
		return err
	}
	if p.Pointer == nil {
		p.Pointer = atomicutil.Pointer[T](&val)
	} else {
		p.Store(&val)
	}
	return nil
}
