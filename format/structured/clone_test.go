package structured

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ---- Clone ---------------------------------------------------------------

func TestClone(t *testing.T) {
	t.Run("scalar_int", func(t *testing.T) {
		v := 42
		c := Clone(v)
		require.Equal(t, v, c)
	})

	t.Run("scalar_string", func(t *testing.T) {
		v := "hello"
		c := Clone(v)
		require.Equal(t, v, c)
	})

	t.Run("map_string_any_nil", func(t *testing.T) {
		var m map[string]any
		c := Clone(m)
		require.Nil(t, c)
	})

	t.Run("map_string_any_empty", func(t *testing.T) {
		m := map[string]any{}
		c := Clone(m)
		require.Equal(t, m, c)
		m["a"] = 1
		require.NotEqual(t, m, c)
	})

	t.Run("map_string_any_flat", func(t *testing.T) {
		m := map[string]any{"a": 1, "b": "two", "c": true}
		c := Clone(m)
		require.Equal(t, m, c)
		c["d"] = "new"
		require.NotContains(t, m, "d")
	})

	t.Run("map_string_any_nested", func(t *testing.T) {
		m := map[string]any{
			"outer": map[string]any{
				"inner": []any{1, 2, 3},
			},
		}
		c := Clone(m)
		require.Equal(t, m, c)
		// mutate nested slice in Clone — original must be unaffected
		c["outer"].(map[string]any)["inner"].([]any)[0] = 99
		require.Equal(t, 1, m["outer"].(map[string]any)["inner"].([]any)[0])
	})

	t.Run("slice_any_nil", func(t *testing.T) {
		var s []any
		c := Clone(s)
		require.Nil(t, c)
	})

	t.Run("slice_any_flat", func(t *testing.T) {
		s := []any{1, "two", true}
		c := Clone(s)
		require.Equal(t, s, c)
		c[0] = 99
		require.Equal(t, 1, s[0])
	})

	t.Run("slice_any_nested", func(t *testing.T) {
		s := []any{[]any{1, 2}, map[string]any{"k": "v"}}
		c := Clone(s)
		require.Equal(t, s, c)
		c[0].([]any)[0] = 99
		require.Equal(t, 1, s[0].([]any)[0])
	})

	t.Run("map_string_string_nil", func(t *testing.T) {
		var m map[string]string
		c := Clone(m)
		require.Nil(t, c)
	})

	t.Run("map_string_string", func(t *testing.T) {
		m := map[string]string{"x": "y"}
		c := Clone(m)
		require.Equal(t, m, c)
		c["z"] = "w"
		require.NotContains(t, m, "z")
	})

	t.Run("slice_byte_nil", func(t *testing.T) {
		var b []byte
		c := Clone(b)
		require.Nil(t, c)
	})

	t.Run("slice_byte", func(t *testing.T) {
		b := []byte{1, 2, 3}
		c := Clone(b)
		require.Equal(t, b, c)
		c[0] = 99
		require.Equal(t, byte(1), b[0])
	})

	t.Run("slice_string_nil", func(t *testing.T) {
		var s []string
		c := Clone(s)
		require.Nil(t, c)
	})

	t.Run("slice_string", func(t *testing.T) {
		s := []string{"a", "b"}
		c := Clone(s)
		require.Equal(t, s, c)
		c[0] = "z"
		require.Equal(t, "a", s[0])
	})

	t.Run("generic_map_via_reflect", func(t *testing.T) {
		// map[string]int hits the reflect path
		m := map[string]int{"a": 1, "b": 2}
		c := Clone(m)
		require.Equal(t, m, c)
		c["c"] = 3
		require.NotContains(t, m, "c")
	})

	t.Run("generic_slice_via_reflect", func(t *testing.T) {
		// []int hits the reflect path
		s := []int{10, 20, 30}
		c := Clone(s)
		require.Equal(t, s, c)
		c[0] = 99
		require.Equal(t, 10, s[0])
	})

	t.Run("struct_by_value", func(t *testing.T) {
		// structs are copied by value (shallow), not deep-cloned
		type S struct{ X int }
		v := S{X: 7}
		c := Clone(v)
		require.Equal(t, v, c)
	})

	t.Run("nil_interface_value", func(t *testing.T) {
		// A named interface type (e.g. type Meta any) with a nil value must not panic. Direct .(T) assertion on a nil
		// any panics in generic code even when T is an interface.
		type Iface any
		var v Iface
		c := Clone(v)
		require.Nil(t, c)
	})
}

// ---- CloneMap ------------------------------------------------------------

func TestCloneMap(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var m map[string]int
		c := CloneMap(m)
		require.Nil(t, c)
	})

	t.Run("empty", func(t *testing.T) {
		m := map[string]int{}
		c := CloneMap(m)
		require.Equal(t, m, c)
		m["a"] = 1
		require.NotEqual(t, m, c)
	})

	t.Run("flat_independent", func(t *testing.T) {
		m := map[string]int{"a": 1, "b": 2}
		c := CloneMap(m)
		require.Equal(t, m, c)
		c["c"] = 3
		require.NotContains(t, m, "c")
	})

	t.Run("string_any_nested_independent", func(t *testing.T) {
		m := map[string]any{"k": []any{1, 2, 3}}
		c := CloneMap(m)
		require.Equal(t, m, c)
		c["k"].([]any)[0] = 99
		require.Equal(t, 1, m["k"].([]any)[0])
	})

	t.Run("preserves_type", func(t *testing.T) {
		type MyMap map[string]int
		m := MyMap{"x": 5}
		c := CloneMap(m)
		require.IsType(t, MyMap{}, c)
		require.Equal(t, m, c)
	})

	t.Run("nil_interface_value", func(t *testing.T) {
		// A map whose value type is a named interface (e.g. type Meta any) must not panic when an entry is nil.
		type Iface any
		m := map[string]Iface{"a": nil}
		c := CloneMap(m)
		require.Nil(t, c["a"])
	})
}

// ---- CloneSlice ----------------------------------------------------------

func TestCloneSlice(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var s []int
		c := CloneSlice(s)
		require.Nil(t, c)
	})

	t.Run("empty", func(t *testing.T) {
		s := []int{}
		c := CloneSlice(s)
		require.NotNil(t, c)
		require.Equal(t, 0, len(c))
	})

	t.Run("flat_independent", func(t *testing.T) {
		s := []int{1, 2, 3}
		c := CloneSlice(s)
		require.Equal(t, s, c)
		c[0] = 99
		require.Equal(t, 1, s[0])
	})

	t.Run("any_nested_independent", func(t *testing.T) {
		s := []any{map[string]any{"k": "v"}}
		c := CloneSlice(s)
		require.Equal(t, s, c)
		c[0].(map[string]any)["k"] = "changed"
		require.Equal(t, "v", s[0].(map[string]any)["k"])
	})

	t.Run("preserves_type", func(t *testing.T) {
		type MySlice []string
		s := MySlice{"a", "b"}
		c := CloneSlice(s)
		require.IsType(t, MySlice{}, c)
		require.Equal(t, s, c)
	})

	t.Run("nil_vs_empty_distinction", func(t *testing.T) {
		require.Nil(t, CloneSlice[[]int](nil))
		require.NotNil(t, CloneSlice([]int{}))
	})

	t.Run("nil_interface_element", func(t *testing.T) {
		// A slice whose element type is a named interface must not panic for nil elements.
		type Iface any
		s := []Iface{nil}
		c := CloneSlice(s)
		require.Nil(t, c[0])
	})
}
