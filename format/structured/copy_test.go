package structured_test

import (
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
				require.NotSame(t, testStruct, val.GetP("struct").Data)
				require.NotSame(t, testStruct, val.GetP("array/4").Data)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := structured.Copy(tt.src, tt.copyFn)

			// fmt.Printf("%s\n%s\n\n", jsonutil.MarshalString(tt.src), jsonutil.MarshalString(res))

			if tt.wantFn == nil {
				require.Equal(t, tt.want, res)
				require.NotSame(t, tt.src, res)
			} else {
				tt.wantFn(res)
			}
		})
	}
}
