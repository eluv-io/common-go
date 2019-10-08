package format

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/qluvio/content-fabric/format/hash"

	"github.com/qluvio/content-fabric/format/id"

	"github.com/qluvio/content-fabric/format/token"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
)

func TestContentDigest(t *testing.T) {
	f := NewTestFactory(t)

	d := f.NewContentDigest(hash.Unencrypted, f.GenerateQID())
	h := fill(t, d)
	assert.NoError(t, h.AssertCode(hash.Q))

	assert.Contains(t, h.String(), "hq__")
}

func TestContentPartDigest(t *testing.T) {
	f := NewTestFactory(t)

	d := f.NewContentPartDigest(hash.Unencrypted)
	h := fill(t, d)
	assert.NoError(t, h.AssertCode(hash.QPart))

	assert.Contains(t, h.String(), "hqp_")
}

func TestIDGenerators(t *testing.T) {
	f := NewTestFactory(t)

	tests := []struct {
		name   string
		id     id.ID
		prefix string
	}{
		{"Account ID", f.GenerateAccountID(), "iacc"},
		{"User ID", f.GenerateUserID(), "iusr"},
		{"QLibID", f.GenerateQLibID(), "ilib"},
		{"QID", f.GenerateQID(), "iq__"},
	}
	for _, v := range tests {
		t.Run(v.name, func(t *testing.T) {
			assertID(t, v.id, v.prefix)
		})
	}
}

func TestTokenGenerators(t *testing.T) {
	f := NewTestFactory(t)

	tests := []struct {
		name   string
		token  token.Token
		prefix string
	}{
		{"QPWriteToken", f.GenerateQPWriteToken(), "tqpw"},
	}
	for _, v := range tests {
		t.Run(v.name, func(t *testing.T) {
			assertToken(t, v.token, v.prefix)
		})
	}
}

func TestHashGenerators(t *testing.T) {
	f := NewTestFactory(t)

	tests := []struct {
		name   string
		md     *hash.Digest
		prefix string
	}{
		{"ContentDigest", f.NewContentDigest(hash.Unencrypted, f.GenerateQID()), "hq__"},
		{"ContentPartDigest", f.NewContentPartDigest(hash.Unencrypted), "hqp_"},
	}
	for _, v := range tests {
		t.Run(v.name, func(t *testing.T) {
			assertDigest(t, v.md, v.prefix)
		})
	}
}

type Generic interface{}

func TestMetadataCodec(t *testing.T) {
	f := NewTestFactory(t)

	jsonData := `
{
  "obj": {
    "k1": "v1",
    "k2": "v2",
    "k3": "v3"
  },
  "int_arr": [ 1, 2, 3 ],
  "big_int_arr": [ 10000000000, 20000000000, 30000000000 ],
  "string_arr": [ "one", "two", "three" ],
  "obj_arr": [
    { "a": 1, "b": 2, "c": 3 },
    { "d": 1, "e": 2, "f": 3 },
    { "g": 1, "h": 2, "i": 3 }
  ]
}
`

	// data := make(map[string]interface{})
	var data Generic
	err := json.Unmarshal([]byte(jsonData), &data)
	require.NoError(t, err)

	c := f.NewMetadataCodec()
	buf := &bytes.Buffer{}
	enc := c.Encoder(buf)
	err = enc.Encode(data)
	require.NoError(t, err)

	fmt.Printf("cbor size: %d\n", buf.Len())

	var decodedData Generic
	dec := c.Decoder(buf)
	err = dec.Decode(&decodedData)
	require.NoError(t, err)

	md1 := marshal(t, &data)
	md2 := marshal(t, &decodedData)
	fmt.Printf("json size: %d\n", len(md1))
	fmt.Println(md1)
	fmt.Println(md2)
	require.Equal(t, md1, md2)
}

func marshal(t *testing.T, data interface{}) string {
	js, err := json.Marshal(data)
	require.NoError(t, err)
	require.NotNil(t, js)
	return string(js)
}

func fill(t *testing.T, d *hash.Digest) *hash.Hash {
	b := make([]byte, 1024)
	c, err := rand.Read(b)
	assert.NoError(t, err)
	assert.Equal(t, 1024, c)
	c, err = d.Write(b)
	assert.NoError(t, err)
	assert.Equal(t, 1024, c)
	h := d.AsHash()
	assert.NotNil(t, h)
	return h
}

func assertID(t *testing.T, id id.ID, expectedPrefix string) {
	fmt.Println(id)
	assert.NotNil(t, id)
	assert.Contains(t, id.String(), expectedPrefix)
}

func assertToken(t *testing.T, tok token.Token, expectedPrefix string) {
	fmt.Println(tok)
	assert.NotNil(t, tok)
	assert.Contains(t, tok.String(), expectedPrefix)
}

func assertDigest(t *testing.T, digest *hash.Digest, expectedPrefix string) {
	n, err := digest.Write([]byte{0, 1, 2, 3, 4, 5})
	assert.Equal(t, 6, n)
	assert.NoError(t, err)
	h := digest.AsHash()
	fmt.Println(h)
	assert.NotNil(t, h)
	assert.Contains(t, h.String(), expectedPrefix)
}
