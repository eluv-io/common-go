package link_test

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/codecs"
	"github.com/eluv-io/common-go/format/encryption"
	"github.com/eluv-io/common-go/format/link"
	"github.com/eluv-io/common-go/util/timeutil"
	"github.com/eluv-io/log-go"
)

func TestBlobLinks(t *testing.T) {
	testData := []byte("test data")
	testDataB64 := base64.StdEncoding.EncodeToString(testData)
	tests := []struct {
		name string
		json string
		lnk  *link.Link
	}{
		{
			name: "no-scheme",
			json: fmt.Sprintf(`{"/":"./blob","data":"%s"}`, testDataB64),
			lnk:  link.NewBlobBuilder().Data(testData).MustBuild(),
		},
		{
			name: "scheme-none",
			json: fmt.Sprintf(`{"/":"./blob","data":"%s","encryption":"%s"}`, testDataB64, encryption.None),
			lnk:  link.NewBlobBuilder().Data(testData).MustBuild(),
		},
		{
			name: "scheme-cgck",
			json: fmt.Sprintf(`{"/":"./blob","data":"%s","encryption":"%s"}`, testDataB64, encryption.ClientGen),
			lnk:  link.NewBlobBuilder().Data(testData).EncryptionScheme(encryption.ClientGen).MustBuild(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var unmarshalled link.Link
			err := json.Unmarshal([]byte(test.json), &unmarshalled)
			require.NoError(t, err)

			expBlob := test.lnk.Blob
			blob := unmarshalled.Blob
			require.Equal(t, expBlob, blob)
			require.Equal(t, expBlob.Data, blob.Data)
			require.Equal(t, expBlob.EncryptionScheme, blob.EncryptionScheme)
			require.Equal(t, expBlob.KID, blob.KID)

			testJSON(t, &unmarshalled, "")
		})
	}
}

func TestBlobLinksInvalid(t *testing.T) {
	testData := []byte("test data")
	testDataB64 := base64.StdEncoding.EncodeToString(testData)
	tests := []struct {
		name string
		json string
	}{
		{
			name: "no data",
			json: `{"/":"./blob"}`,
		},
		{
			name: "data not base64 encoded",
			json: fmt.Sprintf(`{"/":"./blob","data":"%s"}`, testData),
		},
		{
			name: "invalid encryption scheme",
			json: fmt.Sprintf(`{"/":"./blob","data":"%s","encryption":"super-duper-encryption-scheme"}`, testDataB64),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var unmarshalled link.Link
			err := json.Unmarshal([]byte(test.json), &unmarshalled)
			require.Error(t, err)
		})
	}
}

func TestBlobLinksInvalid2(t *testing.T) {
	lnk := link.Link{
		Selector: "blob",
	}
	err := lnk.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "no blob struct")
	fmt.Println(err)
}

func TestBlobLinkCborMarshaling(t *testing.T) {
	var err error
	data := []byte("###blob bytes###")
	lnk := link.NewBlobBuilder().Data(data).EncryptionScheme(encryption.None).MustBuild()

	codec := codecs.NewCborCodec()

	{
		lnkBuf := &bytes.Buffer{}
		err = codec.Encoder(lnkBuf).Encode(lnk)
		require.NoError(t, err)

		var lnkDecoded interface{}
		err = codec.Decoder(lnkBuf).Decode(&lnkDecoded)
		require.NoError(t, err)
		require.Equal(t, *lnk, lnkDecoded)
		require.Equal(t, data, lnkDecoded.(link.Link).Blob.Data)
	}

	{
		blobBuf := &bytes.Buffer{}
		blob := lnk.Blob
		err = codec.Encoder(blobBuf).Encode(blob)
		require.NoError(t, err)

		var blobDecoded link.Blob
		err = codec.Decoder(blobBuf).Decode(&blobDecoded)
		require.NoError(t, err)
		require.Equal(t, *lnk.Blob, blobDecoded)
		require.Equal(t, data, blobDecoded.Data)
	}
}

func TestUnmarshalLargeBlobLink(t *testing.T) {
	data := make([]byte, 10000000)
	_, err := rand.Read(data)
	require.NoError(t, err)

	genLink := map[string]any{
		"blob-slice": map[string]any{
			"/":          "./blob",
			"data":       data,
			"encryption": "none",
		},
		"blob-string": map[string]any{
			"/":          "./blob",
			"data":       base64.StdEncoding.EncodeToString(data),
			"encryption": "none",
		},
	}

	watch := timeutil.StartWatch()
	log.Info("start")
	converted, err := link.ConvertLinks(genLink)
	log.Info("end", "duration", watch.Duration())

	require.NoError(t, err)
	require.Equal(t, data, converted.(map[string]any)["blob-slice"].(*link.Link).Blob.Data)
	require.Equal(t, data, converted.(map[string]any)["blob-string"].(*link.Link).Blob.Data)
}

// TestDecodeSlice demonstrates slow decoding of byte slices with mapstructure
func xTestDecodeSlice(t *testing.T) {
	data := make([]byte, 10000000)
	_, err := rand.Read(data)
	require.NoError(t, err)

	var target []byte

	watch := timeutil.StartWatch()
	log.Info("start")
	//err = codecutil.MapDecode(data, &target)
	err = mapstructure.Decode(data, &target)
	log.Info("end", "duration", watch.Duration())

	require.NoError(t, err)
	require.Equal(t, data, target)
}
