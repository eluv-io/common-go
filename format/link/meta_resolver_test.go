package link_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/qluvio/content-fabric/format/hash"
	"github.com/qluvio/content-fabric/format/link"
	"github.com/qluvio/content-fabric/format/structured"
	"github.com/qluvio/content-fabric/util/jsonutil"
	"github.com/qluvio/content-fabric/util/maputil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mss = map[string]string

func TestResolveMeta(t *testing.T) {
	tests := []struct {
		name string
		src  string
		exp  string
		abs  map[string]string // absolute links: qhash -> metadata
	}{
		{
			name: "rel link to string value",
			src:  `{"a":"one","b":{"/":"./meta/a"}}`,
			exp:  `{"a":"one","b":"one"}`,
		},
		{
			name: "rel link to map",
			src:  `{"a":{"a1":"one","a2":"two"},"b":{"/":"./meta/a"}}`,
			exp:  `{"a":{"a1":"one","a2":"two"},"b":{"a1":"one","a2":"two"}}`,
		},
		{
			name: "rel link chain",
			src:  `{"a":"one","b":{"/":"./meta/c"},"c":{"/":"./meta/a"}}`,
			exp:  `{"a":"one","b":"one","c":"one"}`,
		},
		{
			name: "abs link to string value",
			src:  `{"a":"one","b":{"/":"/qfab/hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7/meta/a"}}`,
			exp:  `{"a":"one","b":"two"}`,
			abs:  mss{"hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7": `{"a":"two"}`},
		},
		{
			name: "multiple abs links",
			src:  `{"a":"one","b":{"/":"/qfab/hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7/meta/a"},"c":{"/":"/qfab/hq__2WiWigdideK39Uvq8e8XziyvPpvkRM16fGnhUpNKJYdGfGLAnAx6AimiZRqRNNLea2TLm/meta/a"}}`,
			exp:  `{"a":"one","b":"two","c":"three"}`,
			abs: mss{
				"hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7": `{"a":"two"}`,
				"hq__2WiWigdideK39Uvq8e8XziyvPpvkRM16fGnhUpNKJYdGfGLAnAx6AimiZRqRNNLea2TLm": `{"a":"three"}`,
			},
		},
		{
			name: "abs link chain",
			src:  `{"a":"one","b":{"/":"/qfab/hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7/meta/a"}}`,
			exp:  `{"a":"one","b":"three"}`,
			abs: mss{
				"hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7": `{"a":{"/":"/qfab/hq__2WiWigdideK39Uvq8e8XziyvPpvkRM16fGnhUpNKJYdGfGLAnAx6AimiZRqRNNLea2TLm/meta/a"}}`,
				"hq__2WiWigdideK39Uvq8e8XziyvPpvkRM16fGnhUpNKJYdGfGLAnAx6AimiZRqRNNLea2TLm": `{"a":"three"}`,
			},
		},
		{
			name: "rel link to sub-tree of abs link",
			src:  `{"a":{"/":"./meta/b/c"},"b":{"/":"/qfab/hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7/meta/a/b"}}`,
			exp:  `{"a":"holdrio","b":{"c":"holdrio"}}`,
			abs:  mss{"hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7": `{"a":{"b":{"c":"holdrio"}}}`},
		},
		{
			name: "rel link to map with link props",
			src:  `{"a":{"a1":"one","a2":"two"},"b":{"/":"./meta/a","a2":"override","a3":"three"}}`,
			exp:  `{"a":{"a1":"one","a2":"two"},"b":{"a1":"one","a2":"override","a3":"three"}}`,
		},
		{
			name: "abs link to map with link props",
			src:  `{"a":"one","b":{"/":"/qfab/hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7/meta","a2":"override","a3":"three"}}`,
			exp:  `{"a":"one","b":{"a1":"one","a2":"override","a3":"three"}}`,
			abs:  mss{"hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7": `{"a1":"one","a2":"two"}`},
		},
		{
			name: "rel link to sub-tree of abs link with props",
			src:  `{"a":{"/":"./meta/b/c"},"b":{"/":"/qfab/hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7/meta/a/b","d":"vd"}}`,
			exp:  `{"a":"holdrio","b":{"c":"holdrio","d":"vd"}}`,
			abs:  mss{"hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7": `{"a":{"b":{"c":"holdrio"}}}`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mp := newTestMetaProvider(test.abs)
			resolver := link.NewMetaResolver(mp)

			src := jsonutil.UnmarshalStringToAny(test.src)
			exp := jsonutil.UnmarshalStringToAny(test.exp)

			src, err := link.ConvertLinks(src)
			assert.NoError(t, err)
			res, err := resolver.ResolveMeta(src, false)
			assert.NoError(t, err)
			assert.EqualValues(t, exp, res)
		})
	}
}

func TestResolveMetaErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		abs  map[string]string
	}{
		{
			name: "invalid path",
			src:  `{"a":{"/":"./meta/b"}}`,
		},
		{
			name: "invalid path - non-existent child of self",
			src:  `{"a":{"/":"./meta/a/b"}}`,
		},
		{
			name: "circular reference - link to root",
			src:  `{"a":{"b":{"/":"./meta/"}}}`,
		},
		{
			name: "circular reference - link to parent",
			src:  `{"a":{"b":{"/":"./meta/a"}}}`,
		},
		{
			name: "circular reference - link to self",
			src:  `{"a":{"b":{"/":"./meta/a/b"}}}`,
		},
		{
			name: "circular reference - link to self 2",
			src:  `{"a":{"/":"./meta/a"}}`,
		},
		{
			name: "circular reference - link to child of self",
			src:  `{"a":{"b":{"/":"./meta/a/b/c"}}}`,
		},
		{
			name: "circular reference - mutual links",
			src:  `{"a":{"/":"./meta/c"},"c":{"/":"./meta/a"}}`,
		},
		{
			name: "circular reference - three links",
			src:  `{"a":{"/":"./meta/b"},"b":{"/":"./meta/c"},"c":{"/":"./meta/a"}}`,
		},
		{
			name: "circular reference - three links with arrays",
			src:  `[{"/":"./meta/1"},{"/":"./meta/2"},{"/":"./meta/0"}]`,
		},
		{
			name: "circular reference in absolute link",
			src:  `{"/":"/qfab/hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7/meta"}`,
			abs: mss{
				"hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7": `{"a":{"/":"./meta/a"}}`,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mp := newTestMetaProvider(test.abs)
			resolver := link.NewMetaResolver(mp)

			src := jsonutil.UnmarshalStringToAny(test.src)

			src, err := link.ConvertLinks(src)
			assert.NoError(t, err)
			res, err := resolver.ResolveMeta(src, false)
			assert.Error(t, err)
			assert.Nil(t, res)
			fmt.Println(err)
		})
	}
}

func BenchmarkLinkResolution_10_5(b *testing.B)   { runLinkResolutionBenchmark(b, 10, 5) }
func BenchmarkLinkResolution_100_5(b *testing.B)  { runLinkResolutionBenchmark(b, 100, 5) }
func BenchmarkLinkResolution_1000_5(b *testing.B) { runLinkResolutionBenchmark(b, 1000, 5) }
func BenchmarkLinkResolution_2000_5(b *testing.B) { runLinkResolutionBenchmark(b, 2000, 5) }

func runLinkResolutionBenchmark(b *testing.B, count, depth int) {
	var err error

	benchmarks := []struct {
		target interface{}
	}{
		{createStruct1(b, count, depth)},
		{createStruct2(b, count, depth)},
		{createStruct3(b, count, depth, true)},
		{createStruct3(b, count, depth, false)},
	}
	for _, bm := range benchmarks {
		b.Run(bm.target.(jm)["name"].(string), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				// b.StopTimer()
				target := deepCopy(bm.target)
				target, err = link.ConvertLinks(target)
				require.NoError(b, err)

				mp := newTestMetaProvider(nil)
				resolver := link.NewMetaResolver(mp)

				// b.StartTimer()
				target, err = resolver.ResolveMeta(target, false)
				require.NoError(b, err)
			}
		})
	}
}
func deepCopy(target interface{}) interface{} {
	var cp interface{}
	jsonutil.Unmarshal(jsonutil.Marshal(target), &cp)
	return cp
}

func TestLargeStructure(t *testing.T) {
	var err error

	debug := false
	count := 1000
	depth := 5

	targets := []interface{}{
		createStruct1(t, count, depth),
		createStruct2(t, count, depth),
		createStruct3(t, count, depth, true),
		createStruct3(t, count, depth, false),
	}

	for _, target := range targets {
		t.Run(fmt.Sprintf("%s_%d_%d", target.(jm)["name"], count, depth), func(t *testing.T) {

			if debug {
				fmt.Println(jsonutil.MarshalString(target))
			}

			measure("convert links", func() {
				target, err = link.ConvertLinks(target)
				require.NoError(t, err)
			})

			mp := newTestMetaProvider(nil)
			resolver := link.NewMetaResolver(mp)

			measure("resolve links", func() {
				target, err = resolver.ResolveMeta(target, false)
				require.NoError(t, err)
				if debug {
					fmt.Println(jsonutil.MarshalString(target))
				}
			})
		})
	}
}

func createStruct1(t require.TestingT, count, depth int) interface{} {
	var err error
	var target interface{}
	target = maputil.From("name", "links to 1 root")
	for i := 0; i < count; i++ {
		path := structured.Path{fmt.Sprintf("top_%d", i)}
		for j := 0; j < depth; j++ {
			path = path.Append(fmt.Sprintf("segment_%d", j))
		}
		if i == 0 {
			target, err = structured.Set(target, path, i)
		} else {
			lnkPath := path.Clone()
			lnkPath[0] = fmt.Sprintf("top_%d", 0) // and adapt it
			target, err = structured.Set(target, path, maputil.From("/", fmt.Sprintf("./meta%s", lnkPath)))
		}
		require.NoError(t, err)
	}
	return target
}

func createStruct2(t require.TestingT, count, depth int) interface{} {
	var err error
	var target interface{}
	target = maputil.From("name", "links to 2 roots")
	for i := 0; i < count; i++ {
		path := structured.Path{fmt.Sprintf("top_%d", i)}
		for j := 0; j < depth; j++ {
			path = path.Append(fmt.Sprintf("segment_%d", j))
		}
		if i == 0 || i == count/2 {
			target, err = structured.Set(target, path, i)
		} else {
			lnkDest := 0
			if i%2 == 0 {
				lnkDest = count / 2
			}
			lnkPath := path.Clone()
			lnkPath[0] = fmt.Sprintf("top_%d", lnkDest) // and adapt it
			target, err = structured.Set(target, path, maputil.From("/", fmt.Sprintf("./meta%s", lnkPath)))
		}
		require.NoError(t, err)
	}
	return target
}

func createStruct3(t require.TestingT, count, depth int, pointToFirst bool) interface{} {
	var err error
	var target interface{}

	name := "single chain"
	if pointToFirst {
		name += " to first"
	} else {
		name += " to last"
	}
	target = maputil.From("name", name)
	for i := 0; i < count; i++ {
		path := structured.Path{fmt.Sprintf("top_%d", i)}
		for j := 0; j < depth; j++ {
			path = path.Append(fmt.Sprintf("segment_%d", j))
		}
		if (i == 0 && pointToFirst) || (i == count-1 && !pointToFirst) {
			target, err = structured.Set(target, path, i)
		} else {
			lnkPath := path.Clone()
			if pointToFirst {
				lnkPath[0] = fmt.Sprintf("top_%d", i-1)
			} else {
				lnkPath[0] = fmt.Sprintf("top_%d", i+1)
			}
			target, err = structured.Set(target, path, maputil.From("/", fmt.Sprintf("./meta%s", lnkPath)))
		}
		require.NoError(t, err)
	}
	return target
}

func measure(msg string, f func()) {
	start := time.Now()
	f()
	d := time.Now().Sub(start)
	fmt.Printf("%s: %s\n", msg, d)
}

// testMetaProvider stores metadata for multiple "other" content objects in a
// map with the content hash as key
type testMetaProvider struct {
	meta map[string]interface{}
}

func newTestMetaProvider(other map[string]string) link.MetaProvider {
	meta := make(map[string]interface{})
	for key, val := range other {
		conv, err := link.ConvertLinks(jsonutil.UnmarshalStringToAny(val))
		if err != nil {
			panic(err)
		}
		meta[key] = conv
	}
	return &testMetaProvider{
		meta: meta,
	}
}

func (t *testMetaProvider) Meta(qhash *hash.Hash, path structured.Path) (interface{}, error) {
	meta := t.meta[qhash.String()]
	return structured.Resolve(path, meta)
}
