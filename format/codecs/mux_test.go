package codecs_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/codecs"
	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/link"
	"github.com/eluv-io/common-go/format/types"
	"github.com/eluv-io/errors-go"
)

func TestMuxCodec(t *testing.T) {
	v1 := TypeV1("test")
	v2 := TypeV2("test")
	v3 := TypeV3{String: "test"}

	bufs := make([]*bytes.Buffer, 2)
	for i, codec := range []codecs.MultiCodec{codecs.JsonMultiCodec, codecs.CborV1MultiCodec} {
		bufs[i] = new(bytes.Buffer)
		encoder := codec.Encoder(bufs[i])
		require.NoError(t, encoder.Encode(v1))
		require.NoError(t, encoder.Encode(v2))
		require.NoError(t, encoder.Encode(v3))
	}

	jsonCborMux := codecs.NewMuxCodec(codecs.CborV1MultiCodec, codecs.JsonMultiCodec)
	for _, buf := range bufs {
		var val1 TypeV1
		var val2 TypeV2
		var val3 TypeV3

		decoder := jsonCborMux.Decoder(buf)
		require.NoError(t, decoder.Decode(&val1))
		require.Equal(t, v1, val1)

		require.NoError(t, decoder.Decode(&val2))
		require.Equal(t, v2, val2)

		require.NoError(t, decoder.Decode(&val3))
		require.Equal(t, v3, val3)
	}

}

type testStruct struct {
	Name string      `json:"name"`
	QID  types.QID   `json:"qid"`
	Hash types.QHash `json:"hash"`
	Link *link.Link  `json:"link"`
}

type testStructV2 struct {
	_    struct{}    `cbor:",toarray"` // encode struct as array
	Name string      `json:"name"`
	QID  types.QID   `json:"qid"`
	Hash types.QHash `json:"hash"`
	Link *link.Link  `json:"link"`
}

// TestVersionedMux tests a typical "upgrade" scenario:
//   - An initial data structure is encoded with the cbor V1 codec.
//   - Then the codec is upgraded to V2 for improved performance and versioning is added for future-proofing.
//   - Then the data structure changes and now gets encoded in a new format (as a CBOR array instead of a map).
//   - Finally, all three versions of the encoded data are decoded with the same CborV2MuxCodec.
//
// In order to make decoding of the initial encoded data work, the CborV2MuxCodec disables versions on the legacy
// CborV1MultiCodec, since that data was encoded without version information.
func TestVersionedMux(t *testing.T) {
	hsh := hash.MustParse("hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7")
	dataV1 := &testStruct{
		Name: "test",
		QID:  id.Generate(id.Q),
		Hash: hsh,
		Link: link.NewBuilder().Selector(link.S.Meta).P("some", "path").AutoUpdate("default").Target(hsh).MustBuild(),
	}
	dataV2 := &testStructV2{
		Name: dataV1.Name,
		QID:  dataV1.QID,
		Hash: dataV1.Hash,
		Link: dataV1.Link,
	}

	bufCbor := new(bytes.Buffer)
	require.NoError(t, codecs.CborV1MultiCodec.Encoder(bufCbor).Encode(dataV1))

	bufCborV2DataV1 := new(bytes.Buffer)
	require.NoError(t, codecs.CborV2MuxCodec.VersionedEncoder(bufCborV2DataV1).EncodeVersioned(1, dataV1))

	bufCborV2DataV2 := new(bytes.Buffer)
	require.NoError(t, codecs.CborV2MuxCodec.VersionedEncoder(bufCborV2DataV2).EncodeVersioned(2, dataV2))

	for i, buf := range []*bytes.Buffer{bufCbor, bufCborV2DataV1, bufCborV2DataV2} {
		fmt.Println(buf.String())
		obj, version, err := codecs.CborV2MuxCodec.VersionedDecoder(buf).DecodeVersioned(
			func(version uint, codec string) interface{} {
				if codec == codecs.CborV1MultiCodec.Header().Path() {
					return &testStruct{}
				}
				switch version {
				case 1:
					return &testStruct{}
				case 2:
					return &testStructV2{}
				}
				return nil
			})
		require.NoError(t, err)
		require.Equal(t, uint(i), version)
		switch i {
		case 0, 1:
			require.Equal(t, dataV1, obj)
		case 2:
			require.Equal(t, dataV2, obj)
		}
	}
}

var twoCodecs = []codecs.MultiCodec{codecs.JsonMultiCodec, codecs.CborV1MultiCodec}

func TestMuxCodecSelect(t *testing.T) {
	buf := &bytes.Buffer{}
	mux := codecs.MuxCodec{
		Codecs: twoCodecs,
		Select: nil,
	}

	t.Run("nil select function", func(t *testing.T) {
		err := mux.Encoder(buf).Encode("test")
		require.NoError(t, err)
		require.Equal(t, string(byte(6))+"/json\n\"test\"\n", buf.String())
	})

	t.Run("select no encoder", func(t *testing.T) {
		mux.Select = func(v interface{}, codecs []codecs.MultiCodec) codecs.MultiCodec {
			return nil
		}
		buf.Reset()
		err := mux.Encoder(buf).Encode("test")
		require.Error(t, err)
		require.Equal(t, "no suitable encoder", errors.Wrap(err).Field("reason"))
	})

}

func TestMuxCodecWrap(t *testing.T) {
	buf := &bytes.Buffer{}
	mux := codecs.NewMuxCodec(twoCodecs...)
	mux.Wrap = true

	err := mux.Encoder(buf).Encode("test")
	require.NoError(t, err)
	require.Equal(t, string(byte(12))+"/multicodec\n"+string(byte(6))+"/json\n\"test\"\n", buf.String())

	var decoded string
	err = mux.Decoder(buf).Decode(&decoded)
	require.NoError(t, err)
	require.Equal(t, "test", decoded)
}

func TestMuxCodecDisableVersions(t *testing.T) {
	buf := &bytes.Buffer{}
	mux := codecs.NewMuxCodec(twoCodecs...).DisableVersions()

	err := mux.VersionedEncoder(buf).EncodeVersioned(7, "test")
	require.NoError(t, err)
	require.Equal(t, string(byte(6))+"/json\n\"test\"\n", buf.String())

	var decoded string
	res, version, err := mux.VersionedDecoder(buf).DecodeVersioned(func(version uint, codec string) interface{} {
		return &decoded
	})
	sres := res.(*string)
	require.NoError(t, err)
	require.Equal(t, "test", decoded)
	require.Equal(t, "test", *sres)
	require.Equal(t, uint(0), version)
}
