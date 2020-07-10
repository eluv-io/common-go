package structured

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGlobCreateFilter(t *testing.T) {
	type args struct {
		selectPaths []Path
		removePaths []Path
	}
	tests := []struct {
		name string
		args args
		want *globFilter
	}{
		{
			name: "select0",
			args: args{
				selectPaths: nil,
			},
			want: gf("", typSelect),
		},
		{
			name: "select0.1",
			args: args{
				selectPaths: []Path{{}},
			},
			want: gf("", typSelect),
		},
		{
			name: "select1",
			args: args{
				selectPaths: []Path{{"a"}},
			},
			want: root(
				gf("a", typSelect),
			),
		},
		{
			name: "select2",
			args: args{
				selectPaths: []Path{{"a"}, {"b", "*", "d"}},
			},
			want: root(
				gf("a", typSelect),
				gf("b", typVoid,
					gf("*", typVoid,
						gf("d", typSelect),
					)),
			),
		},
		{
			name: "select3",
			args: args{
				selectPaths: []Path{{"a", "*", "1"}, {"a", "*", "2"}},
			},
			want: root(
				gf("a", typVoid,
					gf("*", typVoid,
						gf("1", typSelect),
						gf("2", typSelect),
					)),
			),
		},
		{
			name: "select4",
			args: args{
				selectPaths: []Path{{"a", "*", "1"}, {"*"}},
			},
			want: root(
				gf("*", typSelect),
			),
		},
		{
			name: "remove0",
			args: args{
				removePaths: []Path{{}},
			},
			want: gf("", typRemove),
		},
		{
			name: "remove1",
			args: args{
				removePaths: []Path{{"a"}},
			},
			want: gf("", typSelect,
				gf("a", typRemove)),
		},
		{
			name: "remove1.1",
			args: args{
				removePaths: []Path{{"*"}},
			},
			want: gf("", typSelect,
				gf("*", typRemove)),
		},
		{
			name: "remove2",
			args: args{
				removePaths: []Path{{"a"}, {"b", "*", "d"}},
			},
			want: gf("", typSelect,
				gf("a", typRemove),
				gf("b", typSelect,
					gf("*", typSelect,
						gf("d", typRemove),
					),
				)),
		},
		{
			name: "remove3",
			args: args{
				removePaths: []Path{{"a", "*", "1"}, {"a", "*", "2"}},
			},
			want: gf("", typSelect,
				gf("a", typSelect,
					gf("*", typSelect,
						gf("1", typRemove),
						gf("2", typRemove),
					),
				)),
		},
		{
			name: "rs1",
			args: args{
				selectPaths: []Path{{"a", "*", "1"}, {"a", "*", "2"}},
				removePaths: []Path{{"a", "*", "2"}, {"a", "*", "3"}},
			},
			want: root(
				gf("a", typVoid,
					gf("*", typVoid,
						gf("1", typSelect),
						gf("2", typRemove),
						gf("3", typRemove),
					)),
			),
		},
		{
			name: "rs2",
			args: args{
				selectPaths: []Path{{"a", "*", "1"}, {"a", "*", "2"}},
				removePaths: []Path{{"a", "*", "*"}},
			},
			want: root(
				gf("a", typVoid,
					gf("*", typVoid,
						gf("*", typRemove),
					)),
			),
		},
		{
			name: "rs3",
			args: args{
				selectPaths: []Path{ParsePath("/public/asset_metadata")},
				removePaths: []Path{ParsePath("/public/asset_metadata/titles/*/*/assets")},
			},
			want: root(
				gf("public", typVoid,
					gf("asset_metadata", typSelect,
						gf("titles", typSelect,
							gf("*", typSelect,
								gf("*", typSelect,
									gf("assets", typRemove),
								),
							),
						),
					)),
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := createFilter(tt.args.selectPaths, tt.args.removePaths)
			require.EqualValues(t, tt.want, got)
		})
	}
}

func root(children ...*globFilter) *globFilter {
	return gf("", typVoid, children...)
}

func gf(seg string, typ filterType, children ...*globFilter) *globFilter {
	res := &globFilter{
		seg: seg,
		typ: typ,
	}
	for _, child := range children {
		if res.children == nil {
			res.children = map[string]*globFilter{}
		}
		res.children[child.seg] = child
	}
	return res
}
