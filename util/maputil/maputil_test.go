package maputil

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSortedKeys(t *testing.T) {
	type args struct {
		m map[string]interface{}
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "nil",
			args: args{
				m: nil,
			},
			want: []string{},
		},
		{
			name: "empty",
			args: args{
				m: map[string]interface{}{},
			},
			want: []string{},
		},
		{
			name: "m1",
			args: args{
				m: map[string]interface{}{
					"k1": "v1",
					"k2": "v2",
					"k3": "v3",
					"k4": "v4",
				},
			},
			want: []string{"k1", "k2", "k3", "k4"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, SortedKeys(tt.args.m))
			require.Equal(t, tt.want, SortedStringKeys(tt.args.m))
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
				m:              someMap,
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
