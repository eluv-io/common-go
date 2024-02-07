package volatile

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVolatile(t *testing.T) {
	thing := NewThing("bob", 123, 456)
	thing.U.Store(456)
	thing.V.Store(789)
	bb, err := json.Marshal(thing)
	require.NoError(t, err)
	fmt.Println(string(bb))

	t2 := NewThing("", 0, 0)
	err = json.Unmarshal(bb, t2)
	require.NoError(t, err)
	require.Equal(t, thing.Name, t2.Name)
	require.Equal(t, thing.U.Load(), t2.U.Load())
	require.Equal(t, thing.V.Load(), t2.V.Load())
}

type Thing struct {
	Name string  `json:"name"`
	U    *Uint64 `json:"uint"`
	V    *Int64  `json:"int"`
}

func NewThing(name string, u uint64, v int64) *Thing {
	return &Thing{
		Name: name,
		U:    NewUint64(u),
		V:    NewInt64(v),
	}
}
