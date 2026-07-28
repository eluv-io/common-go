package maputil

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSortedKeys(t *testing.T) {
	type args struct {
		m map[string]string
	}
	tests := []struct {
		name             string
		args             args
		wantSortedKeys   []string             // sorted keys
		wantSortedValues []string             // sorted values
		wantSorted       []string             // values sorted by keys
		wantSortedPairs  []KV[string, string] // sorted KV pairs
	}{
		{
			name: "nil",
			args: args{
				m: nil,
			},
			wantSortedKeys:   []string{},
			wantSortedValues: []string{},
			wantSorted:       []string{},
			wantSortedPairs:  []KV[string, string]{},
		},
		{
			name: "empty",
			args: args{
				m: map[string]string{},
			},
			wantSortedKeys:   []string{},
			wantSortedValues: []string{},
			wantSorted:       []string{},
			wantSortedPairs:  []KV[string, string]{},
		},
		{
			name: "map1",
			args: args{
				m: map[string]string{
					"k1": "v2",
					"k2": "v3",
					"k3": "v4",
					"k4": "v1",
				},
			},
			wantSortedKeys:   []string{"k1", "k2", "k3", "k4"},
			wantSortedValues: []string{"v1", "v2", "v3", "v4"},
			wantSorted:       []string{"v2", "v3", "v4", "v1"},
			wantSortedPairs:  []KV[string, string]{{"k1", "v2"}, {"k2", "v3"}, {"k3", "v4"}, {"k4", "v1"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantSortedKeys, SortedKeys(tt.args.m))
			require.Equal(t, tt.wantSortedKeys, SortedStringKeys(tt.args.m))
			require.Equal(t, tt.wantSortedValues, SortedValues(tt.args.m))
			require.Equal(t, tt.wantSorted, Sorted(tt.args.m))
			require.Equal(t, tt.wantSortedPairs, SortedPairs(tt.args.m))
		})
	}
}

func TestFrom(t *testing.T) {
	type args struct {
		nameValuePairs []interface{}
	}
	tests := []struct {
		name string
		args args
		want map[string]interface{}
	}{
		{
			name: "nil",
			args: args{
				nameValuePairs: nil,
			},
			want: nil,
		},
		{
			name: "empty",
			args: args{
				nameValuePairs: []interface{}{},
			},
			want: nil,
		},
		{
			name: "nil value",
			args: args{
				nameValuePairs: []interface{}{"k1", nil},
			},
			want: map[string]interface{}{
				"k1": nil,
			},
		},
		{
			name: "pairs",
			args: args{
				nameValuePairs: []interface{}{"k1", "v1", "k2", "v2", "k3", "v3", "k4", "v4"},
			},
			want: map[string]interface{}{
				"k1": "v1",
				"k2": "v2",
				"k3": "v3",
				"k4": "v4",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, From(tt.args.nameValuePairs...))
		})
	}
}

var emptyMap = make(map[string]interface{})
var emptySlice = make([]interface{}, 0)
var someMap = map[string]interface{}{
	"s1": "v1",
	"s2": "v2",
}

func TestAdd(t *testing.T) {
	type args struct {
		m              map[string]interface{}
		nameValuePairs []interface{}
	}
	tests := []struct {
		name string
		args args
		want map[string]interface{}
	}{
		{
			name: "nil-nil",
			args: args{
				m:              nil,
				nameValuePairs: nil,
			},
			want: nil,
		},
		{
			name: "empty-empty",
			args: args{
				m:              emptyMap,
				nameValuePairs: emptySlice,
			},
			want: emptyMap,
		},
		{
			name: "*-nil",
			args: args{
				m:              someMap,
				nameValuePairs: emptySlice,
			},
			want: someMap,
		},
		{
			name: "nil-*",
			args: args{
				m:              nil,
				nameValuePairs: []interface{}{"k1", "v1", "k2", "v2"},
			},
			want: map[string]interface{}{
				"k1": "v1",
				"k2": "v2",
			},
		},
		{
			name: "normal",
			args: args{
				m:              map[string]interface{}{"s1": "v1", "s2": "v2"},
				nameValuePairs: []interface{}{"k1", "v1", "k2", "v2"},
			},
			want: map[string]interface{}{
				"k1": "v1",
				"k2": "v2",
				"s1": "v1",
				"s2": "v2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Add(tt.args.m, tt.args.nameValuePairs...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Add() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCopyMSI(t *testing.T) {
	ms := CopyMSI("s")
	require.Equal(t, 0, len(ms))

	m := map[string]interface{}{
		"un":   1,
		"deux": "two",
	}
	m2 := CopyMSI(m)
	require.Equal(t, m, m2)

	type any struct {
		name string
	}
	ma := map[string]*any{
		"one": {name: "one"},
		"two": {name: "two"},
	}
	ma2 := CopyMSI(ma)
	require.Equal(t, 2, len(ma2))
	require.Equal(t, ma2["one"], ma["one"])
	require.Equal(t, ma2["two"], ma["two"])
}

func TestCopy(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var m map[string]string
		require.Nil(t, Copy(m))
	})

	t.Run("empty", func(t *testing.T) {
		m := map[string]string{}
		cp := Copy(m)
		require.NotNil(t, cp)
		require.Empty(t, cp)
	})

	t.Run("string map", func(t *testing.T) {
		m := map[string]string{"k1": "v1", "k2": "v2"}
		cp := Copy(m)
		require.Equal(t, m, cp)
		m["k1"] = "changed"
		m["k3"] = "new"
		require.Equal(t, "v1", cp["k1"])
		require.NotContains(t, cp, "k3")
	})

	t.Run("int map", func(t *testing.T) {
		m := map[int]int{1: 10, 2: 20, 3: 30}
		cp := Copy(m)
		require.Equal(t, m, cp)
		m[1] = 99
		delete(m, 2)
		require.Equal(t, 10, cp[1])
		require.Contains(t, cp, 2)
	})

	t.Run("independent copy", func(t *testing.T) {
		m := map[string]string{"k1": "v1", "k2": "v2"}
		cp := Copy(m)
		cp["k1"] = "changed"
		cp["k3"] = "new"
		require.Equal(t, "v1", m["k1"])
		require.NotContains(t, m, "k3")
		m["k2"] = "also changed"
		m["k4"] = "also new"
		require.Equal(t, "v2", cp["k2"])
		require.NotContains(t, cp, "k4")
	})

	t.Run("shallow copy shares pointers", func(t *testing.T) {
		type val struct{ n int }
		v1 := &val{n: 1}
		v2 := &val{n: 2}
		m := map[string]*val{"a": v1, "b": v2}
		cp := Copy(m)
		require.Same(t, m["a"], cp["a"])
		require.Same(t, m["b"], cp["b"])
	})

	t.Run("custom map type", func(t *testing.T) {
		type strMap map[string]string
		m := strMap{"k1": "v1", "k2": "v2"}
		cp := Copy(m)
		require.IsType(t, strMap{}, cp)
		require.Equal(t, m, cp)
		m["k1"] = "changed"
		require.Equal(t, "v1", cp["k1"])
	})

	t.Run("struct key", func(t *testing.T) {
		type point struct{ x, y int }
		type pointMap map[point]string
		m := pointMap{{1, 2}: "a", {3, 4}: "b"}
		cp := Copy(m)
		require.IsType(t, pointMap{}, cp)
		require.Equal(t, m, cp)
		m[point{1, 2}] = "changed"
		m[point{5, 6}] = "new"
		require.Equal(t, "a", cp[point{1, 2}])
		require.NotContains(t, cp, point{5, 6})
	})
}

func TestClear(t *testing.T) {
	{
		m := map[string]string{"k1": "v1", "k2": "v2"}
		Clear(m)
		require.Empty(t, m)
	}
	{
		m := map[int]int{1: 10, 2: 20}
		Clear(m)
		require.Empty(t, m)
	}
	{
		type point struct{ x, y int }
		m := map[point]string{{1, 2}: "a", {3, 4}: "b"}
		Clear(m)
		require.Empty(t, m)
	}
}
