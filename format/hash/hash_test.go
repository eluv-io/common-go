package hash_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"

	"eluvio/format/hash"
	"eluvio/format/id"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var hsh *hash.Hash

const hashString = "hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7"

func init() {
	htype := hash.Type{hash.Q, hash.Unencrypted}
	digest, _ := hex.DecodeString("9cbc07c3f991725836a3aa2a581ca2029198aa420b9d99bc0e131d9f3e2cbe47")
	size := int64(1024)
	idx, _ := id.FromString("iq__WxoChT9EZU2PRdTdNU7Ldf")

	hsh = &hash.Hash{Type: htype, Digest: digest, Size: size, ID: idx}
}

func TestHashCtorError(t *testing.T) {
	idx := id.NewID(id.Q, []byte{1, 2, 3, 4})
	digest := make([]byte, sha256.Size)
	rand.Read(digest)
	h1, err := hash.New(
		hash.Type{Code: hash.Q, Format: hash.Unencrypted},
		digest,
		1234,
		idx)
	require.NoError(t, err)
	h2, err := hash.FromString(h1.String())
	require.NoError(t, err)
	require.Equal(t, h1, h2)

	_, err = hash.New(
		hash.Type{Code: hash.Q, Format: hash.AES128AFGH},
		digest,
		1234,
		id.NewID(id.QLib, []byte{1, 2, 3, 4}))
	require.Error(t, err)

	_, err = hash.New(
		hash.Type{Code: hash.QPart, Format: hash.AES128AFGH},
		digest,
		1234,
		idx)
	require.Error(t, err)

	_, err = hash.New(
		hash.Type{Code: hash.Q, Format: hash.AES128AFGH},
		make([]byte, 3),
		1234,
		idx)
	require.Error(t, err)

	hp1 := &hash.Hash{
		Type:   hash.Type{Code: hash.QPart, Format: hash.AES128AFGH},
		Digest: digest,
		ID:     idx,
		Size:   1234,
	}
	hp2, err := hash.FromString(hp1.String())
	require.NoError(t, err)       // String() does not serialize id: no error
	require.NotEqual(t, hp1, hp2) // but not equal
	hp1.ID = nil
	require.Equal(t, hp1, hp2)
}

func TestStringConversion(t *testing.T) {
	hshString := hsh.String()
	assert.Equal(t, hashString, hshString)

	hshFromString, err := hash.FromString(hshString)
	assert.NoError(t, err)

	assert.Equal(t, hsh, hshFromString)
	assert.Equal(t, hshString, fmt.Sprint(hsh))
	assert.Equal(t, hshString, fmt.Sprintf("%v", hsh))
	assert.Equal(t, "blub"+hshString, fmt.Sprintf("blub%s", hsh))

	idx := id.NewID(id.Q, []byte{1, 2, 3, 4})
	digest := make([]byte, sha256.Size)
	rand.Read(digest)
	h2 := &hash.Hash{
		Type:   hash.Type{Code: hash.Q, Format: hash.Unencrypted},
		Digest: digest,
		ID:     idx,
		Size:   1234,
	}
	h3, err := hash.FromString(h2.String())
	require.NoError(t, err)

	require.Equal(t, h2.Digest, h3.Digest)
	require.Equal(t, h2.Size, h3.Size)
	require.Equal(t, h2.Type, h3.Type)
	require.Equal(t, h2.ID, h3.ID)
	require.Equal(t, h2, h3)

}

func TestInvalidStringConversions(t *testing.T) {
	tests := []struct {
		hash string
	}{
		// PENDING(LUK): Check why "" is accepted and returns nil!
		// {hash: ""},
		{hash: "blub"},
		{hash: "hq__"},
		{hash: "hq__1111"},
		{hash: "hq__ "},
		{hash: "hq__QmYtUc4iTCbbfVSDNKvtQqrfyezPPnFvE33wFmutw9PBB"},
	}
	for _, test := range tests {
		t.Run(test.hash, func(t *testing.T) {
			h, err := hash.FromString(test.hash)
			assert.Error(t, err)
			assert.Nil(t, h)
		})
	}
}

func ExampleConversions() {

	hashString := "hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7"
	fmt.Println("hash", "string", hashString)

	// Convert a hash string to a hash object
	h, _ := hash.FromString(hashString)
	fmt.Println("hash", "object", h)

	// Extract the data of the hash object
	fmt.Println("hash", "type", h.Type)
	fmt.Println("hash", "digest", h.Digest)
	fmt.Println("hash", "size", h.Size)
	fmt.Println("hash", "id", h.ID)

	// Convert the raw bytes to a hash object
	h2, _ := hash.New(h.Type, h.Digest, h.Size, h.ID)
	fmt.Println("hash", "from data", h2)

	// Output:
	// hash string hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7
	// hash object hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7
	// hash type hq__
	// hash digest [156 188 7 195 249 145 114 88 54 163 170 42 88 28 162 2 145 152 170 66 11 157 153 188 14 19 29 159 62 44 190 71]
	// hash size 1024
	// hash id iq__WxoChT9EZU2PRdTdNU7Ldf
	// hash from data hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7
}

func TestJSON(t *testing.T) {
	b, err := json.Marshal(hsh)
	assert.NoError(t, err)
	assert.Equal(t, "\""+hashString+"\"", string(b))

	var unmarshalled *hash.Hash
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, hsh, unmarshalled)
}

type Wrapper struct {
	Hash *hash.Hash
}

func TestWrappedJSON(t *testing.T) {
	s := Wrapper{
		Hash: hsh,
	}
	b, err := json.Marshal(s)
	assert.NoError(t, err)
	assert.Contains(t, string(b), hashString)

	fmt.Println(string(b))

	var unmarshalled Wrapper
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, s, unmarshalled)
}

func TestCreation(t *testing.T) {
	digest, _ := hex.DecodeString("9cbc07c3f991725836a3aa2a581ca2029198aa420b9d99bc0e131d9f3e2cbe47")
	id, _ := id.FromString("iq__WxoChT9EZU2PRdTdNU7Ldf")
	h, err := hash.New(hash.Type{hash.Q, hash.Unencrypted}, digest, 1024, id)
	assert.NoError(t, err)
	assertHash(t, h, "hq_")
	//assertHash(t, GenerateAccountHash(), "acc")
	//assertHash(t, GenerateUserHash(), "usr")
	//assertHash(t, GenerateQLibHash(), "lib")
	//assertHash(t, GenerateQHash(), "q__")
}

func assertHash(t *testing.T, hsh *hash.Hash, expectedPrefix string) {
	fmt.Println(hsh)
	assert.NotNil(t, hsh)
	assert.Contains(t, hsh.String(), expectedPrefix)
}

func TestDigest(t *testing.T) {
	idx, _ := id.FromString("iq__WxoChT9EZU2PRdTdNU7Ldf")

	d := hash.NewDigest(sha256.New(), hash.Type{hash.Q, hash.Unencrypted}, idx)
	b := make([]byte, 1024)

	c, err := rand.Read(b)
	assert.NoError(t, err)
	assert.Equal(t, 1024, c)

	c, err = d.Write(b)
	assert.NoError(t, err)
	assert.Equal(t, 1024, c)

	h := d.AsHash()
	assert.NotNil(t, h)
	assert.NoError(t, h.AssertCode(hash.Q))

	fmt.Println(h)
}

func TestEmptyDigest(t *testing.T) {
	idx, _ := id.FromString("iq__WxoChT9EZU2PRdTdNU7Ldf")

	d := hash.NewDigest(sha256.New(), hash.Type{hash.Q, hash.Unencrypted}, idx)
	h := d.AsHash()
	assert.NotNil(t, h)
	assert.NoError(t, h.AssertCode(hash.Q))

	fmt.Println(h)
}

func TestEqual(t *testing.T) {
	require.True(t, hsh.Equal(hsh))

	other, err := hash.FromString(hsh.String())
	require.NoError(t, err)
	require.True(t, hsh.Equal(other))
	require.True(t, other.Equal(hsh))

	require.False(t, hsh.Equal(nil))
	require.False(t, hsh.Equal(&hash.Hash{}))

	var nilHash *hash.Hash
	require.False(t, nilHash.Equal(hsh))
	require.True(t, nilHash.Equal(nil))
}
