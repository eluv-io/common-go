package link_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"eluvio/format/encryption"
	"eluvio/format/link"

	"github.com/stretchr/testify/require"
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

			expBlob, err := test.lnk.AsBlob()
			require.NoError(t, err)

			blob, err := unmarshalled.AsBlob()
			require.NoError(t, err)

			require.Equal(t, expBlob, blob)
			require.Equal(t, expBlob.Data, blob.Data)
			require.Equal(t, expBlob.EncryptionScheme, blob.EncryptionScheme)

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
	err := lnk.Validate(true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no data")
	fmt.Println(err)
}
