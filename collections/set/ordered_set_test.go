package set_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/collections/set"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/util/sliceutil"
)

func TestOrderedSet(t *testing.T) {
	doTestOrderedSet(t, set.NewOrderedSet[string](), "", "a", "b", "c", "d", "e")
	doTestOrderedSet(t, set.NewOrderedSet[int](), 1, 2, 3, 4, 5, 6, 7, 8, 9)
	doTestOrderedSet(t, set.NewOrderedSet[float64](), 1.1, 2.2, 3.3, 4.4, 5.5, 6.6, 7.7, 8.9, 9.9)
	doTestOrderedSet(t,
		set.NewOrderedSetFn[id.ID](
			func(e1, e2 id.ID) int {
				return sliceutil.Compare(e1, e2)
			}),
		id.Generate(id.Q),
		id.Generate(id.Q),
		id.Generate(id.Q),
		id.Generate(id.Q),
	)
}

func doTestOrderedSet[T any](t *testing.T, s *set.OrderedSet[T], values ...T) {
	shuffle := func(i, j int) {
		values[i], values[j] = values[j], values[i]
	}
	rand.Shuffle(len(values), shuffle)
	s2 := s.Clone()
	s2.Clear()
	s3 := s.Clone()
	s3.Clear()

	t.Run(fmt.Sprint(values), func(t *testing.T) {
		for _, value := range values {
			require.False(t, s.Contains(value))
		}

		insert := func() {
			for i, value := range values {
				require.Equal(t, i, s.Size())
				s.Insert(value)
				for j, value2 := range values {
					require.Equal(t, j <= i, s.Contains(value2), i, j, value2)
				}
				require.Equal(t, i+1, s.Size())
			}
		}

		t.Run("Insert", func(t *testing.T) {
			insert()
		})

		// JSON marshal & unmarshal
		t.Run("JSON", func(t *testing.T) {
			bts, err := json.Marshal(s)
			require.NoError(t, err)

			err = json.Unmarshal(bts, s2)
			require.NoError(t, err)
			for i, value := range values {
				require.True(t, s2.Contains(value), "index %d value %v", i, value)
			}
		})

		// unmarshal unordered JSON array with duplicate elements
		t.Run("JSON unmarshal duplicates", func(t *testing.T) {
			buf := createUnorderedJSONArrayWithDuplicates(t, values)
			err := json.Unmarshal(buf, s3)
			require.NoError(t, err)
			for i, value := range values {
				require.True(t, s3.Contains(value), "index %d value %v", i, value)
			}
		})

		// CBOR marshal & unmarshal
		t.Run("CBOR", func(t *testing.T) {
			bts, err := cbor.Marshal(s)
			require.NoError(t, err)

			err = cbor.Unmarshal(bts, s2)
			require.NoError(t, err)
			for i, value := range values {
				require.True(t, s2.Contains(value), "index %d value %v", i, value)
			}
		})

		t.Run("Clear", func(t *testing.T) {
			s.Clear()
			require.Equal(t, 0, s.Size())
		})

		insert()
		rand.Shuffle(len(values), shuffle)

		t.Run("Remove", func(t *testing.T) {
			for i, value := range values {
				require.Equal(t, len(values)-i, s.Size())
				s.Remove(value)
				for j, value2 := range values {
					require.Equal(t, j > i, s.Contains(value2), i, j, value2)
				}
				require.Equal(t, len(values)-i-1, s.Size())
			}
		})
	})
}

func createUnorderedJSONArrayWithDuplicates[T any](t *testing.T, values []T) []byte {
	buf := bytes.Buffer{}
	buf.WriteString("[")
	for i := 0; i < 2; i++ {
		for j, value := range values {
			if !(i == 0 && j == 0) {
				buf.WriteString(",")
			}
			v, err := json.Marshal(value)
			require.NoError(t, err)
			buf.Write(v)
		}
	}
	buf.WriteString("]")
	return buf.Bytes()
}

func TestOrderedSet_InsertDuplicates(t *testing.T) {
	s := set.NewOrderedSet(1, 2, 2, 1, 1, 2)
	require.Equal(t, 2, s.Size())
	require.True(t, s.Contains(1))
	require.True(t, s.Contains(2))
}

func TestOrderedSet_RemoveNonexisting(t *testing.T) {
	s := set.NewOrderedSet[int]()
	require.Equal(t, 0, s.Size())
	s.Remove(1, 1, 1, 2, 3, 1)
	require.Equal(t, 0, s.Size())
}
