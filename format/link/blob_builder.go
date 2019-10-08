package link

import (
	"encoding/base64"

	"github.com/qluvio/content-fabric/format/encryption"
)

// NewBlobBuilder creates a link builder that can be used to build a blob link:
//   lnk, err := link.NewBlobBuilder().Data(data).Build()
//   lnk, err := link.NewBlobBuilder().EncryptionScheme(encryption.ClientGen).Data(encryptedData).Build()
func NewBlobBuilder() *BlobBuilder {
	b := &BlobBuilder{
		b: NewBuilder(),
	}
	b.b.Selector(S.Blob)
	return b
}

type BlobBuilder struct {
	b *Builder
}

func (b *BlobBuilder) EncryptionScheme(scheme encryption.Scheme) *BlobBuilder {
	b.b.AddProp("encryption", scheme.String())
	return b
}

func (b *BlobBuilder) Data(data []byte) *BlobBuilder {
	b.b.AddProp("data", base64.StdEncoding.EncodeToString(data))
	return b
}

func (b *BlobBuilder) Build() (*Link, error) {
	return b.b.Build()
}

func (b *BlobBuilder) MustBuild() *Link {
	return b.b.MustBuild()
}
