package link_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/encryption"
	"github.com/eluv-io/common-go/format/link"
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

			require.NotNil(t, lnk.Blob.Data)
			require.Equal(t, test.expData, lnk.Blob.Data)
			require.Equal(t, test.expScheme, lnk.Blob.EncryptionScheme)
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
