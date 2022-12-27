package format_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format"
	"github.com/eluv-io/common-go/format/encryption"
	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/keys"
	"github.com/eluv-io/common-go/format/link"
)

// TestMarshalUnmarshalCurrent marshals and unmarshals the current test data.
func TestMarshalUnmarshalCurrent(t *testing.T) {
	jsonData, cborData := marshal(t, testData)
	unmarshalAndValidate(t, jsonData, cborData, testData)
}

// TestUnmarshalRegression validates unmarshaling all recorded snapshots in the
// `testdata/unmarshal_regression_test_snapshots` directory.
//
// New snapshots can be created as follows:
//  1. add a new snapshot to `snapshots`
//  2. if needed:
//     a) add a new DataStructVx struct and dataVx variable
//     b) Assign the new dataVx to testData
//  3. rename xTestCreateSnapshot to TestCreateSnapshot
//  4. run TestCreateSnapshot:
//     go test -run="^TestCreateSnapshot$" ./format
//  5. run TestUnmarshalRegression to verify that all snapshots including the new one pass successfully:
//     go test -run="^TestUnmarshalRegression$" ./format
//  6. rename TestCreateSnapshot back to xTestCreateSnapshot
//  7. commit the new snapshot and modifications of this file
func TestUnmarshalRegression(t *testing.T) {
	for _, snap := range snapshots {
		t.Run(snap.dir, func(t *testing.T) {
			jsonData, err := os.ReadFile(filepath.Join(snap.dir, "data.json"))
			require.NoError(t, err)
			cborData, err := os.ReadFile(filepath.Join(snap.dir, "data.cbor"))
			require.NoError(t, err)
			unmarshalAndValidate(t, jsonData, cborData, snap.data)
		})
	}
}

var snapshots = []snapshot{
	{
		dir:  "testdata/unmarshal_regression_test/v1",
		data: dataV1,
	},
	{
		// 2023-01-12: no type changes, new test links
		dir:  "testdata/unmarshal_regression_test/v1_2",
		data: dataV2,
	},
	{
		// 2023-01-12:
		//  * use github.com/fxamacker/cbor/v2, deprecate github.com/ugorji/go/codec
		//  * more streamlined serialization for links
		dir:  "testdata/unmarshal_regression_test/v2_2",
		data: dataV2,
	},
}

func xTestCreateSnapshot(t *testing.T) {
	var err error
	jsonData, cborData := marshal(t, testData)

	snap := snapshots[len(snapshots)-1]

	targetDir := snap.dir
	err = os.MkdirAll(targetDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(targetDir, "data.json"), jsonData, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(targetDir, "data.cbor"), cborData, 0755)
	require.NoError(t, err)
}

type snapshot struct {
	// the directory where the snapshot is stored
	dir string
	// the data structure expected on unmarshal
	data interface{}
}

type DataStructV1 struct {
	Hashes []*hash.Hash
	IDs    []id.ID
	Keys   []keys.Key
	Links  []*link.Link
}

type DataStructV2 struct {
	*DataStructV1
	Links2 []*link.Link
}

// the most recent version of test data to marshal/unmarshal
var testData = dataV2

var dataV2 = &DataStructV2{
	DataStructV1: dataV1,
	Links2: []*link.Link{
		link.NewBuilder().
			Selector(link.S.Rep).
			Target(hash.MustParse("hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7")).
			P("some", "rep", "call").
			AutoUpdate("default").
			AddProp("key1", "val1").
			AddProp("key2", "val2").
			AddProp("key2", "val2").
			MustBuild(),
		link.NewBuilder().
			Selector(link.S.File).
			Target(hash.MustParse("hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7")).
			P("some", "file").
			Off(100).
			Len(444).
			MustBuild(),
	},
}

// first version of test data
var dataV1 = &DataStructV1{
	Hashes: []*hash.Hash{
		hash.MustParse("hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7"),
		hash.MustParse("hqpedYvWGgmzmerRxa2Rzv6dqjDogfCZE7dwSuDnfgaSfGbMeXXnT"),
		hash.MustParse("hql_7TmHLg49Qd4NtgfcPeWKAG7fsk7HujeMHaE33Bwm2kLYDdjqYJw"),
	},
	IDs: []id.ID{
		id.MustParse("iacc1W7LcTy7"),
		id.MustParse("inod2fsjuJhzMUo6zhj1Nm7DPA"),
		id.MustParse("iusrVCFjezf8Pry4kUKU1MpY75"),
	},
	Keys: []keys.Key{
		keys.MustParse("kpec21qpqGEr7h2sBvpshR98rp75HrG8ipXxXroUJYE7gCggrB"),
		keys.MustParse("kped77f3gGVKqVm8ituv6harnvMX85H3YPnQ8GBDTmZbiRDX"),
		keys.MustParse("kpsr4ikBWWzn8edKSULEwPNfrvMoD4vA1onMUb88PtpCHDDH"),
		keys.MustParse("kpblAF9e8Q5n37HWZaeL5oq8dNrwVbW9Xvd5A4JewKhkWgMYkT6ZertGEnm1MpmLjLbQHE"),
	},
	Links: []*link.Link{
		link.NewBuilder().
			Selector(link.S.Meta).
			P("public", "name").
			AutoUpdate("default").
			AddProp("some", "prop").
			MustBuild(),
		link.NewBuilder().
			Selector(link.S.Meta).
			Target(hash.MustParse("hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7")).
			P("public", "name").
			MustBuild(),
		link.NewBlobBuilder().
			Data(blobData).
			EncryptionScheme(encryption.None).
			MustBuild(),
		link.NewBlobBuilder().
			Data(blobData).
			EncryptionScheme(encryption.ClientGen).
			MustBuild(),
	},
}

var (
	blobData  = []byte("some blob data")
	cborCodec = format.NewFactory().NewMetadataCodec()
)

func marshal(t require.TestingT, data interface{}) ([]byte, []byte) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)

	cborBuf := &bytes.Buffer{}
	err = cborCodec.Encoder(cborBuf).Encode(data)
	require.NoError(t, err)
	cborData := cborBuf.Bytes()

	return jsonData, cborData
}

func unmarshalAndValidate(t *testing.T, jsonData []byte, cborData []byte, wantData interface{}) {
	var jsonUnmarshaled = newInstance(wantData)
	err := json.Unmarshal(jsonData, jsonUnmarshaled)
	require.NoError(t, err)

	var cborUnmarshaled = reflect.New(reflect.TypeOf(wantData).Elem()).Interface()

	// for inspection at http://cbor.me
	// fmt.Println(hex.EncodeToString(cborData[bytes.IndexByte(cborData, '\n')+1:]))

	decoder := cborCodec.Decoder(bytes.NewReader(cborData))
	err = decoder.Decode(cborUnmarshaled)
	require.NoError(t, err)

	require.Equal(t, wantData, jsonUnmarshaled)
	require.Equal(t, wantData, cborUnmarshaled)
}

func newInstance(data interface{}) any {
	return reflect.New(reflect.TypeOf(data).Elem()).Interface()
}

/*
## v1_2
goos: darwin
goarch: amd64
pkg: github.com/eluv-io/common-go/format
cpu: Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz
BenchmarkCBORMarshal
BenchmarkCBORMarshal-16    	   98467	     12136 ns/op	    6520 B/op	      81 allocs/op

## v2_2
goos: darwin
goarch: amd64
pkg: github.com/eluv-io/common-go/format
cpu: Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz
BenchmarkCBORMarshal
BenchmarkCBORMarshal-16    	  122457	      9787 ns/op	    3950 B/op	      40 allocs/op
*/
func BenchmarkCBORMarshal(b *testing.B) {
	b.ReportAllocs()
	cborBuf := &bytes.Buffer{}
	for i := 0; i < b.N; i++ {
		cborBuf.Reset()
		err := cborCodec.Encoder(cborBuf).Encode(testData)
		require.NoError(b, err)
	}
}

/*
## v1_2
goos: darwin
goarch: amd64
pkg: github.com/eluv-io/common-go/format
cpu: Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz
BenchmarkCBORUnmarshal
BenchmarkCBORUnmarshal-16    	   31647	     36735 ns/op	   17401 B/op	     306 allocs/op

## v2_2
goos: darwin
goarch: amd64
pkg: github.com/eluv-io/common-go/format
cpu: Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz
BenchmarkCBORUnmarshal
BenchmarkCBORUnmarshal-16    	   41112	     29356 ns/op	   11945 B/op	     179 allocs/op
*/
func BenchmarkCBORUnmarshal(b *testing.B) {
	_, cborData := marshal(b, testData)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cborUnmarshaled := newInstance(testData)
		decoder := cborCodec.Decoder(bytes.NewReader(cborData))
		err := decoder.Decode(cborUnmarshaled)
		require.NoError(b, err)
	}
}
