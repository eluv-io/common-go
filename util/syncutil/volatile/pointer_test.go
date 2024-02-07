package volatile

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPointer(t *testing.T) {
	a := &thing{A: 222, B: "hello"}
	pa := NewPointer[thing](a)
	bb, err := json.Marshal(pa)
	require.NoError(t, err)

	pa2 := NewPointer[thing](&thing{})
	err = json.Unmarshal(bb, pa2)
	require.NoError(t, err)

	require.Equal(t, pa.Load(), pa2.Load())
}

type thing struct {
	A int    `json:"a"`
	B string `json:"b"`
}
