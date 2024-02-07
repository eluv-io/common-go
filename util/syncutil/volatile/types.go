package volatile

import (
	"encoding/json"
	"sync/atomic"

	"github.com/eluv-io/common-go/util/syncutil/atomicutil"
)

type Uint64 struct {
	*atomic.Uint64
}

func NewUint64(val uint64) *Uint64 {
	return &Uint64{Uint64: atomicutil.Uint64(val)}
}

func (v *Uint64) MarshalJSON() ([]byte, error) {
	val := v.Load()
	return json.Marshal(&val)
}

func (v *Uint64) UnmarshalJSON(b []byte) error {
	var val uint64
	err := json.Unmarshal(b, &val)
	if err != nil {
		return err
	}
	if v.Uint64 == nil {
		v.Uint64 = atomicutil.Uint64(val)
	} else {
		v.Store(val)
	}
	return nil
}

func (v *Uint64) MarshalBinary() (data []byte, err error) {
	return v.MarshalJSON()
}
func (v *Uint64) UnmarshalBinary(data []byte) error {
	return v.UnmarshalJSON(data)
}

type Int64 struct {
	*atomic.Int64
}

func NewInt64(val int64) *Int64 {
	return &Int64{Int64: atomicutil.Int64(val)}
}

func (v *Int64) MarshalJSON() ([]byte, error) {
	val := v.Load()
	return json.Marshal(&val)
}

func (v *Int64) UnmarshalJSON(b []byte) error {
	var val int64
	err := json.Unmarshal(b, &val)
	if err != nil {
		return err
	}
	if v.Int64 == nil {
		v.Int64 = atomicutil.Int64(val)
	} else {
		v.Store(val)
	}
	return nil
}

//func (v *Int64) MarshalBinary() (data []byte, err error) {
//	return v.MarshalJSON()
//}
//func (v *Int64) UnmarshalBinary(data []byte) error {
//	return v.UnmarshalJSON(data)
//}
