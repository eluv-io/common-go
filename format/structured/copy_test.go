package structured_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/structured"
)

type copyTestStruct struct {
	name  string
	value int
}

func TestCopy(t *testing.T) {
	testStruct := &copyTestStruct{
		name:  "Name",
		value: 101,
	}
	src := map[string]interface{}{
		"string": "a string",
		"bool":   true,
		"int":    99,
		"float":  3.1415926,
		"struct": testStruct,
		"map": map[string]interface{}{
			"k1": "v1",
			"k2": "v2",
			"k3": "v3",
		},
		"array": []interface{}{
			"a string",
			false,
			99,
			3.1415926,
			testStruct,
		},
	}

	tests := []struct {
		name   string
		src    interface{}
		copyFn structured.CopyFn
		want   interface{}
		wantFn func(res interface{}) // custom result check function
	}{
		{
			name: "nil",
			src:  nil,
			want: nil,
		},
		{
			name: "nil",
			src:  (map[string]interface{})(nil),
			want: (map[string]interface{})(nil),
		},
		{
			name: "basic types",
			src:  src,
			want: src,
		},
		{
			name: "noop copy fn",
			src:  src,
			copyFn: func(val interface{}) (override bool, newVal interface{}) {
				return false, nil
			},
			want: src,
		},
		{
			name: "custom copy fn",
			src:  src,
			copyFn: func(val interface{}) (override bool, newVal interface{}) {
				return true, 77
			},
			want: 77,
		},
		{
			name: "bool switch",
			src:  src,
			copyFn: func(val interface{}) (override bool, newVal interface{}) {
				switch typ := val.(type) {
				case bool:
					return true, !typ
				}
				return false, nil
			},
			want: func() interface{} {
				cpy := structured.Copy(src).(map[string]interface{})
				cpy["bool"] = false
				cpy["array"].([]interface{})[1] = true
				return cpy
			}(),
		},
		{
			name: "custom struct copy",
			src:  src,
			copyFn: func(val interface{}) (override bool, newVal interface{}) {
				switch typ := val.(type) {
				case *copyTestStruct:
					return true, &copyTestStruct{
						name:  typ.name,
						value: typ.value,
					}
				}
				return false, nil
			},
			wantFn: func(res interface{}) {
				val := structured.Wrap(res)
				requireNotSame(t, testStruct, val.GetP("struct").Data)
				requireNotSame(t, testStruct, val.GetP("array/4").Data)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := structured.Copy(tt.src, tt.copyFn)

			// fmt.Printf("%s\n%s\n\n", jsonutil.MarshalString(tt.src), jsonutil.MarshalString(res))

			if tt.wantFn == nil {
				require.Equal(t, tt.want, res)
				requireNotSame(t, tt.src, res)
			} else {
				tt.wantFn(res)
			}
		})
	}
}

// requireNotSame is the same as a previous version of require.NotSame. Since 1.10.0, require.NotSame checks that both
// arguments are actually pointers and otherwise fails. That's not what we want here.
func requireNotSame(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) {
	samePointers := func(first, second interface{}) bool {
		firstPtr, secondPtr := reflect.ValueOf(first), reflect.ValueOf(second)
		if firstPtr.Kind() != reflect.Ptr || secondPtr.Kind() != reflect.Ptr {
			return false // not both are pointers
		}

		firstType, secondType := reflect.TypeOf(first), reflect.TypeOf(second)
		if firstType != secondType {
			return false // both are pointers, but of different types
		}

		// compare pointer addresses
		return first == second
	}

	if samePointers(expected, actual) {
		require.Fail(t, fmt.Sprintf(
			"Expected and actual point to the same object: %p %#[1]v",
			expected), msgAndArgs...)
	}

}
