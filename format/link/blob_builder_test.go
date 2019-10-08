package link_test

import (
	"fmt"
	"testing"

	"eluvio/format/encryption"
	"eluvio/format/link"

	"github.com/stretchr/testify/require"
)

func TestBlobLinkBuilder(t *testing.T) {
	data := []byte("test data")
	tests := []struct {
		builder   *link.BlobBuilder
		expData   []byte
		expScheme encryption.Scheme
	}{
		{
			builder:   link.NewBlobBuilder().EncryptionScheme(encryption.None).Data(data),
			expData:   data,
			expScheme: encryption.None,
		},
		{
			builder:   link.NewBlobBuilder().EncryptionScheme(encryption.ClientGen).Data(data),
			expData:   data,
			expScheme: encryption.ClientGen,
		},
	}
	for idx, test := range tests {
		t.Run(fmt.Sprintf("blob-%d", idx), func(t *testing.T) {
			lnk, err := test.builder.Build()
			require.NoError(t, err)
			require.Equal(t, "./blob", lnk.String())

			blob, err := lnk.AsBlob()
			require.NoError(t, err)
			require.Equal(t, test.expData, blob.Data)
			require.Equal(t, test.expScheme, blob.EncryptionScheme)
		})
	}
}

func TestBlobLinkBuilderInvalid(t *testing.T) {
	tests := []struct {
		builder *link.BlobBuilder
	}{
		{
			builder: link.NewBlobBuilder(),
		},
		{
			builder: link.NewBlobBuilder().Data([]byte{}).EncryptionScheme(encryption.UNKNOWN),
		},
	}
	for idx, test := range tests {
		t.Run(fmt.Sprintf("blob-%d", idx), func(t *testing.T) {
			_, err := test.builder.Build()
			require.Error(t, err)
			fmt.Println(err)
		})
	}
}
