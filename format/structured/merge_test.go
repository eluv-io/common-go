package structured

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/eluv-io/common-go/util/jsonutil"

	"github.com/stretchr/testify/require"
)

func TestMerge(t *testing.T) {
	tests := []struct {
		name    string
		target  string        // json
		path    string        // e.g. /my/path
		source  string        // json
		sources []string      // json
		opts    *MergeOptions // optional
		want    string        // json
	}{
		{
			name:   "merge nil into nil",
			target: `null`,
			path:   "/",
			source: `null`,
			want:   `null`,
		},
		{
			name:   "merge string into nil",
			target: `null`,
			path:   "/",
			source: `"a string"`,
			want:   `"a string"`,
		},
		{
			name:   "merge map into nil",
			target: `null`,
			path:   "/",
			source: `{"a":"va"}`,
			want:   `{"a":"va"}`,
		},
		{
			name:   "merge map into empty map",
			target: `{}`,
			path:   "/",
			source: `{"a":"va"}`,
			want:   `{"a":"va"}`,
		},
		{
			name:   "merge int into nil with path",
			target: `null`,
			path:   "/path/to/create",
			source: `99`,
			want:   `{"path":{"to":{"create":99}}}`,
		},
		{
			name:   "merge map into nil with path",
			target: `null`,
			path:   "/path/to/create",
			source: `{"a":"va"}`,
			want:   `{"path":{"to":{"create":{"a":"va"}}}}`,
		},
		{
			name:   "overwrite map entry",
			target: `{"a":"va"}`,
			path:   "/",
			source: `{"a":"va-new"}`,
			want:   `{"a":"va-new"}`,
		},
		{
			name:   "replace map entry (string)",
			target: `{"a":"va"}`,
			path:   "/a",
			source: `"va-new"`,
			want:   `{"a":"va-new"}`,
		},
		{
			name:   "merge map entry (map)",
			target: `{"a":{"b":"vb"}}`,
			path:   "/a",
			source: `{"c":"vc"}`,
			want:   `{"a":{"b":"vb","c":"vc"}}`,
		},
		{
			name:   "merge map into map (add)",
			target: `{"a":"va"}`,
			path:   "/",
			source: `{"b":"vb"}`,
			want:   `{"a":"va","b":"vb"}`,
		},
		{
			name:    "merge multiple maps into map (add)",
			target:  `{"a":"va"}`,
			path:    "/",
			sources: []string{`{"b":"vb"}`, `{"c":"vc"}`},
			want:    `{"a":"va","b":"vb","c":"vc"}`,
		},
		{
			name:   "replace map entry with map",
			target: `{"a":"va"}`,
			path:   "/a",
			source: `{"b":"vb"}`,
			want:   `{"a":{"b":"vb"}}`,
		},
		{
			name:   "delete map entry",
			target: `{"a":"va"}`,
			path:   "/",
			source: `{"a":null}`,
			want:   `{}`,
		},
		{
			name:   "merge array into empty array",
			target: `[]`,
			path:   "/",
			source: `["a"]`,
			want:   `["a"]`,
		},
		{
			name:   "merge array into array",
			target: `["a","b"]`,
			path:   "/",
			source: `["c","d"]`,
			want:   `["a","b","c","d"]`,
		},
		{
			name:   "merge array into array with duplicates",
			target: `["a","b"]`,
			path:   "/",
			source: `["b","c"]`,
			want:   `["a","b","c"]`,
		},
		{
			name:   "merge array into array with duplicates - append",
			target: `["a","b"]`,
			path:   "/",
			source: `["b","c"]`,
			want:   `["a","b","b","c"]`,
			opts:   &MergeOptions{MakeCopy: true, ArrayMergeMode: ArrayMergeModes.Append()},
		},
		{
			name:   "merge array with duplicates into array with duplicates",
			target: `{"arr":["a","b","a"]}`,
			path:   "/arr",
			source: `["b","c"]`,
			want:   `{"arr":["a","b","a","c"]}`,
		},
		{
			name:   "merge array with duplicates into array with duplicates - dedupe",
			target: `{"arr":["a","b","a"]}`,
			path:   "/arr",
			source: `["b","c"]`,
			want:   `{"arr":["a","b","c"]}`,
			opts:   &MergeOptions{MakeCopy: true, ArrayMergeMode: ArrayMergeModes.Dedupe()},
		},
		{
			name:   "merge array into array with duplicates - replace",
			target: `["a","b"]`,
			path:   "/",
			source: `["b","c"]`,
			want:   `["b","c"]`,
			opts:   &MergeOptions{MakeCopy: true, ArrayMergeMode: ArrayMergeModes.Replace()},
		},
		{
			name:   "replace array entry",
			target: `["a","b"]`,
			path:   "/1",
			source: `"c"`,
			want:   `["a","c"]`,
		},
		{
			name:   "replace array entry",
			target: `["a","b"]`,
			path:   "/0",
			source: `["b","c"]`,
			want:   `[["b","c"],"b"]`,
		},
		{
			name:   "replace root array with string",
			target: `["a","b"]`,
			path:   "/",
			source: `"string"`,
			want:   `"string"`,
		},
		{
			name:   "replace array with string",
			target: `{"k":["a","b"]}`,
			path:   "/",
			source: `{"k":"string"}`,
			want:   `{"k":"string"}`,
		},
		{
			name:   "replace map with string",
			target: `{"k":{"a":"va","b":"vb"}}`,
			path:   "/",
			source: `{"k":"string"}`,
			want:   `{"k":"string"}`,
		},
		{
			name:   "complex example",
			target: `{"a":"va","b":["one","two"],"c":"vc","d":{"e":"ve"}}`,
			path:   "/",
			source: `{"a":"new va","b":["three","four"],"c":null,"d":"vd"}`,
			want:   `{"a":"new va","b":["one","two","three","four"],"d":"vd"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+"|path["+tt.path+"]", func(t *testing.T) {
			var tgt, tgt2, exp interface{}
			var sources []interface{}
			require.NoError(t, json.Unmarshal([]byte(tt.target), &tgt))
			require.NoError(t, json.Unmarshal([]byte(tt.target), &tgt2))
			require.NoError(t, json.Unmarshal([]byte(tt.want), &exp))
			if len(tt.sources) == 0 {
				tt.sources = append(tt.sources, tt.source)
			}
			for _, s := range tt.sources {
				var src interface{}
				require.NoError(t, json.Unmarshal([]byte(s), &src))
				sources = append(sources, src)
			}

			require.NotNil(t, sources)

			fmt.Println(jsonutil.MarshalString(&tgt))
			if len(sources) > 0 {
				fmt.Println(jsonutil.MarshalString(&sources[0]))
			}

			sourcesJson := jsonutil.MarshalCompactString(sources)

			var res interface{}
			var err error

			if tt.opts != nil {
				res, err = MergeWithOptions(*tt.opts, tgt, ParsePath(tt.path, "/"), sources...)
			} else {
				res, err = MergeCopy(tgt, ParsePath(tt.path, "/"), sources...)
			}
			require.NoError(t, err)
			require.Equal(t, exp, res)
			if tt.opts == nil || tt.opts.MakeCopy {
				// tgt unchanged!
				require.Equal(t, tgt2, tgt)
				// sources unchanged!
				require.Equal(t, sourcesJson, jsonutil.MarshalCompactString(sources))
			}

			fmt.Println(jsonutil.MarshalString(&res))
		})
	}
}

func TestMerge2(t *testing.T) {
	res, err := Merge(nil, Path{"a", "b"}, "just a string")
	require.NoError(t, err)
	require.EqualValues(t, jm{"a": jm{"b": "just a string"}}, res)
}

func TestMergeAtPath(t *testing.T) {
	tests := [][2]interface{}{
		{
			jm{"a": jm{"b": jm{"c": "val1"}}},
			jm{"a": jm{"b": jm{"c": "val2"}}},
		},
		{
			jm{"a": jm{"b": jm{"c": jm{"d": jm{"e": jm{"f": jm{"g": jm{"h": "val1"}}}}}}}},
			jm{"a": jm{"b": jm{"c": jm{"d": jm{"e": jm{"f": jm{"g": jm{"h": jm{"i": jm{"j": jm{"k": "val2"}}}}}}}}}}},
		},
		{
			jm{"a": jm{"b": jm{"c": jm{"d": jm{"e": jm{"f": jm{"g": jm{"h": jm{"i": jm{"j": jm{"k": "val2"}}}}}}}}}}},
			jm{"a": jm{"b": jm{"c": jm{"d": jm{"e": jm{"f": jm{"g": jm{"h": "val1"}}}}}}}},
		},
	}

	for _, test := range tests {
		m1 := test[0]
		m2 := test[1]
		m1Copy := jsonutil.MustClone(m1)
		m2Copy := jsonutil.MustClone(m2)

		res, err := MergeWithOptions(MergeOptions{MakeCopy: true}, m1, nil, m2)
		require.NoError(t, err)
		require.EqualValues(t, m2, res)
		require.Equal(t, m1Copy, m1) // src unchanged!
		require.Equal(t, m2Copy, m2) // src unchanged!

		res, err = Merge(m1, nil, m2)
		require.NoError(t, err)
		require.EqualValues(t, m2, res)
	}
}
