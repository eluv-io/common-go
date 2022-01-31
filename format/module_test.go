package format

import (
	"testing"

	"github.com/eluv-io/common-go/format/hash"

	"github.com/eluv-io/inject-go"
	"github.com/stretchr/testify/assert"
)

func TestModule(t *testing.T) {
	f := NewTestFactory(t)

	f.NewContentDigest(hash.Unencrypted, f.GenerateQID())
}

func NewTestFactory(t *testing.T) Factory {
	inj := NewTestInjector(t)
	fobj, err := inj.Get((*Factory)(nil))
	assert.NoError(t, err)
	f, ok := fobj.(Factory)
	assert.True(t, ok)
	assert.NotNil(t, f)
	return f
}

func NewTestInjector(t *testing.T) inject.Injector {
	inj, err := inject.NewInjector(NewModule())
	assert.NoError(t, err)
	return inj
}
