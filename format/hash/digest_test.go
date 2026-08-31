package hash_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
)

func TestDigest(t *testing.T) {
	idx, _ := id.FromString("iq__WxoChT9EZU2PRdTdNU7Ldf")

	d := hash.NewBuilder()
	b := make([]byte, 1024)

	n, err := rand.Read(b)
	assert.NoError(t, err)
	assert.Equal(t, 1024, n)

	n, err = d.Write(b)
	assert.NoError(t, err)
	assert.Equal(t, 1024, n)

	h, err := d.BuildHash()
	assert.NoError(t, err)
	assert.NotNil(t, h)
	assert.Equal(t, hash.QPart, h.Type.Code)
	assert.Equal(t, hash.Unencrypted, h.Type.Format)

	h, err = h.AsContentHash(idx)
	assert.NoError(t, err)
	assert.NoError(t, h.AssertCode(hash.Q))

	assert.NotNil(t, h)
	assert.NoError(t, h.AssertCode(hash.Q))

	fmt.Println(h)
}

func TestEmptyDigest(t *testing.T) {
	idx, _ := id.FromString("iq__WxoChT9EZU2PRdTdNU7Ldf")

	h, err := hash.NewBuilder().BuildHash()
	assert.NoError(t, err)
	assert.NotNil(t, h)

	fmt.Println(h)

	h, err = h.AsContentHash(idx)
	assert.NoError(t, err)
	assert.NoError(t, h.AssertCode(hash.Q))

	fmt.Println(h)
	fmt.Println(h.Describe())
}
