package link

import (
	"github.com/eluv-io/common-go/format/encryption"
)

// NewBlobBuilder creates a link builder that can be used to build a blob link:
//
//	lnk, err := link.NewBlobBuilder().Data(data).Build()
//	lnk, err := link.NewBlobBuilder().EncryptionScheme(encryption.ClientGen).Data(encryptedData).Build()
func NewBlobBuilder() *BlobBuilder {
	b := &BlobBuilder{
		b: NewBuilder(),
	}
	b.b.Selector(S.Blob)
	b.b.l.Blob = &Blob{
		EncryptionScheme: encryption.None,
	}
	return b
}

type BlobBuilder struct {
	b *Builder
}

func (b *BlobBuilder) EncryptionScheme(scheme encryption.Scheme) *BlobBuilder {
	b.b.l.Blob.EncryptionScheme = scheme
	return b
}

func (b *BlobBuilder) Data(data []byte) *BlobBuilder {
	b.b.l.Blob.Data = data
	return b
}

func (b *BlobBuilder) KID(kid string) *BlobBuilder {
	b.b.l.Blob.KID = kid
	return b
}

func (b *BlobBuilder) ReplaceProps(p map[string]interface{}) *BlobBuilder {
	b.b.ReplaceProps(p)
	return b
}

func (b *BlobBuilder) AddProps(p map[string]interface{}) *BlobBuilder {
	b.b.AddProps(p)
	return b
}

func (b *BlobBuilder) AddProp(key string, val interface{}) *BlobBuilder {
	b.b.AddProp(key, val)
	return b
}

func (b *BlobBuilder) Build() (*Link, error) {
	return b.b.Build()
}

func (b *BlobBuilder) MustBuild() *Link {
	return b.b.MustBuild()
}
