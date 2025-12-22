package codecutil_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/eluv-io/common-go/format"
	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/link"
	"github.com/eluv-io/common-go/format/token"
	"github.com/eluv-io/common-go/util/codecutil"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/stringutil"

	"github.com/stretchr/testify/require"
)

func TestMapDecode(t *testing.T) {
	tests := []struct {
		name    string
		src     interface{}
		dst     interface{}
		wantErr bool
	}{
		{
			name:    "ID",
			src:     id.Generate(id.Q).String(),
			dst:     &id.ID{},
			wantErr: false,
		},
		{
			name:    "Hash",
			src:     "hq__EKjpzYq4vjPxchdoSm8fUSvK2y3PYVgLPdMWP8yqRRvu4rBnv3BY1BS7pdjVjfvvsasaTZA9qq",
			dst:     &hash.Hash{},
			wantErr: false,
		},
		{
			name:    "Link",
			src:     link.NewBuilder().Selector(link.S.Meta).P("some", "path").MustBuild().String(),
			dst:     &link.Link{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := codecutil.MapDecode(tt.src, tt.dst)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.src, fmt.Sprintf("%s", tt.dst))
			}
		})
	}
}

type testStruct struct {
	String string       `json:"string"`
	Int    int          `json:"int"`
	ID     id.ID        `json:"id"`
	Hash   hash.Hash    `json:"hash"`
	Token  *token.Token `json:"token"`
	Link   link.Link    `json:"link"`
	Bytes  []byte       `json:"bytes"`
}

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

func TestMapDecodeStruct(t *testing.T) {
	hsh, err := hash.FromString("hq__EKjpzYq4vjPxchdoSm8fUSvK2y3PYVgLPdMWP8yqRRvu4rBnv3BY1BS7pdjVjfvvsasaTZA9qq")
	require.NoError(t, err)

	qid := id.Generate(id.Q)
	nid := id.Generate(id.QNode)
	tok, err := token.NewObject(token.QWrite, qid, nid)
	require.NoError(t, err)
	ts := testStruct{
		stringutil.RandomString(10),
		rnd.Intn(1000000),
		qid,
		*hsh,
		tok,
		*link.NewBuilder().Selector(link.S.Meta).P("some", "path").AddProp("custom", "prop").MustBuild(),
		[]byte{1, 2, 3},
	}

	jsonText := jsonutil.Marshal(ts)
	src := jsonutil.UnmarshalToAny(jsonText)

	dst := testStruct{}

	err = codecutil.MapDecode(src, &dst)
	require.NoError(t, err)
	require.Equal(t, &ts, &dst)
}

// Tests decoding to a struct using a CBOR blob as source in order to make sure
// custom types also work when they are present as custom types in the source
// map (vs. a generic string of map representation when unmarshaled from JSON).
func TestMapDecodeStructCBOR(t *testing.T) {
	hsh, err := hash.FromString("hq__EKjpzYq4vjPxchdoSm8fUSvK2y3PYVgLPdMWP8yqRRvu4rBnv3BY1BS7pdjVjfvvsasaTZA9qq")
	require.NoError(t, err)

	ts := testStruct{
		stringutil.RandomString(10),
		rnd.Int(),
		id.Generate(id.Q),
		*hsh,
		token.Generate(token.QWrite),
		*link.NewBuilder().Selector(link.S.Meta).P("some", "path").AddProp("custom", "prop").MustBuild(),
		[]byte{1, 2, 3},
	}

	codec := format.NewFactory().NewMetadataCodec()

	buf := &bytes.Buffer{}
	err = codec.Encoder(buf).Encode(&ts)
	require.NoError(t, err)

	var src interface{}
	err = codec.Decoder(buf).Decode(&src)
	require.NoError(t, err)

	dst := testStruct{}
	err = codecutil.MapDecode(src, &dst)
	require.NoError(t, err)
	// force token string update for comparison
	_ = dst.Token.String()
	require.Equal(t, &ts, &dst)
}

type innerColor struct {
	Color string `json:"color"`
}
type innerType struct {
	Type string `json:"type"`
}
type upper struct {
	innerColor
	innerType
	Name string `json:"name"`
	Size int    `json:"size"`
}

type upperP struct {
	*innerColor
	*innerType
	Name string `json:"name"`
	Size int    `json:"size"`
}

type upperSimple struct {
	innerColor
	innerType
}

func TestMapDecodeSquash(t *testing.T) {
	{
		in0 := &upper{
			innerColor: innerColor{Color: "red"},
			innerType:  innerType{Type: "car"},
			Name:       "x",
			Size:       2,
		}
		bb, err := json.Marshal(in0)
		require.NoError(t, err)
		var msi interface{}
		err = json.Unmarshal(bb, &msi)
		require.NoError(t, err)

		in1 := &upper{}
		err = codecutil.MapDecode(msi, in1)
		require.NoError(t, err)
		//fmt.Println(jsonutil.MarshalCompactString(in1))
		//{"color":"","type":"","name":"x","size":2}
		require.Equal(t, "", in1.Color)
		require.Equal(t, "", in1.Type)

		in1 = &upper{}
		err = codecutil.MapDecode(msi, in1, true)
		require.NoError(t, err)
		//fmt.Println(jsonutil.MarshalCompactString(in1))
		//{"color":"red","type":"car","name":"x","size":2}
		require.Equal(t, in0, in1)
	}
	{
		in0 := &upperP{
			innerColor: &innerColor{Color: "red"},
			innerType:  &innerType{Type: "car"},
			Name:       "x",
			Size:       2,
		}
		bb, err := json.Marshal(in0)
		require.NoError(t, err)
		//{"color":"red","type":"car","name":"x","size":2}
		var msi interface{}
		err = json.Unmarshal(bb, &msi)
		require.NoError(t, err)

		in1 := &upperP{}
		err = codecutil.MapDecode(msi, in1)
		require.NoError(t, err)
		//fmt.Println(jsonutil.MarshalCompactString(in1))
		//{"name":"x","size":2}
		require.Nil(t, in1.innerColor)
		require.Nil(t, in1.innerType)

		in1 = &upperP{}
		err = codecutil.MapDecode(msi, in1, true)
		require.NoError(t, err)
		//fmt.Println(jsonutil.MarshalCompactString(in1))
		//{"name":"x","size":2}
		require.Nil(t, in1.innerColor)
		require.Nil(t, in1.innerType)

		in1 = &upperP{
			innerColor: &innerColor{},
			innerType:  &innerType{},
		}
		err = codecutil.MapDecode(msi, in1, true)
		require.NoError(t, err)
		//fmt.Println(jsonutil.MarshalCompactString(in1))
		//{"color":"red","type":"car","name":"x","size":2}
		require.Equal(t, in0, in1)
	}
	{
		in0 := &upperSimple{
			innerColor: innerColor{Color: "red"},
			innerType:  innerType{Type: "car"},
		}
		bb, err := json.Marshal(in0)
		require.NoError(t, err)
		//fmt.Println(jsonutil.MarshalCompactString(in0))
		//{"color":"red","type":"car"}
		var msi interface{}
		err = json.Unmarshal(bb, &msi)
		require.NoError(t, err)

		in1 := &upperSimple{}
		err = codecutil.MapDecode(msi, in1)
		require.NoError(t, err)
		//fmt.Println(jsonutil.MarshalCompactString(in1))
		//{"color":"","type":""}
		require.Equal(t, "", in1.Color)
		require.Equal(t, "", in1.Type)

		in1 = &upperSimple{}
		err = codecutil.MapDecode(msi, in1, true)
		require.NoError(t, err)
		//fmt.Println(jsonutil.MarshalCompactString(in1))
		//{"color":"red","type":"car"}
		require.Equal(t, "red", in1.Color)
		require.Equal(t, "car", in1.Type)

		require.Equal(t, in0, in1)
	}
}

type idStr string

func (i *idStr) UnmarshalText(text []byte) error {
	*i = idStr(text)
	return nil
}

type simple struct {
	Id idStr `json:"id"`
}

// BenchmarkMapDecode
// using MethodByName in decodeHook and 'old' DecodeHookExec in mapstructure
// BenchmarkMapDecode-8   	  291985	      3868 ns/op	     576 B/op	      20 allocs/op
// using MethodByName in decodeHook and 'new' DecodeHookExec in mapstructure
// BenchmarkMapDecode-8   	  393734	      2745 ns/op	     576 B/op	      20 allocs/op
// using cast in decodeHook and 'new' DecodeHookExec in mapstructure
// BenchmarkMapDecode-8   	  670012	      1677 ns/op	     368 B/op	      12 allocs/op
func BenchmarkMapDecode(b *testing.B) {

	in0 := &simple{
		Id: "x",
	}
	bb, err := json.Marshal(in0)
	require.NoError(b, err)
	var msi interface{}
	err = json.Unmarshal(bb, &msi)
	require.NoError(b, err)

	target := &simple{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err = codecutil.MapDecode(msi, target)
		require.NoError(b, err)
	}
}
