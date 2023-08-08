package codecutil_test

import (
	"bytes"
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
	require.Equal(t, &ts, &dst)
}
