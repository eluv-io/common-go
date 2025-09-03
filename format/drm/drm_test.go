package drm_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/errors-go"

	"github.com/eluv-io/common-go/format/drm"
	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
)

var key *drm.KeyID

const keyString = "drm_2qU32EeeHhVBxMF9vC8ABUsF5rmbfY1a6TvUGr3EYpYv1EvAc1FAM9tkACxiYmPoPxaEfbTNzHsAVh4TyKGJEFCd1gM"

var khash *hash.Hash

const khashString = "hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7"

func init() {
	htype := hash.Type{Code: hash.Q, Format: hash.Unencrypted}
	hdigest, _ := hex.DecodeString("9cbc07c3f991725836a3aa2a581ca2029198aa420b9d99bc0e131d9f3e2cbe47")
	hsize := int64(1024)
	hid, _ := id.FromString("iq__WxoChT9EZU2PRdTdNU7Ldf")
	khash = &hash.Hash{Type: htype, Digest: hdigest, Size: hsize, ID: hid}
	kcode := drm.Key
	kid, _ := hex.DecodeString("1157591206bf888e839c69bd1c9fa54d")
	key = &drm.KeyID{Code: kcode, ID: kid, Hash: khash}
}

func TestConstructor(t *testing.T) {
	hdigest := make([]byte, sha256.Size)
	rand.Read(hdigest)
	hid := id.NewID(id.Q, []byte{1, 2, 3, 4})
	khash := &hash.Hash{Type: hash.Type{Code: hash.Q, Format: hash.Unencrypted}, Digest: hdigest, ID: hid, Size: 1234}
	kid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	k1, err := drm.New(drm.Key, kid, khash)
	require.NoError(t, err)
	k2, err := drm.FromString(k1.String())
	require.NoError(t, err)
	require.Equal(t, k1, k2)

	_, err = drm.New(255, kid, khash)
	require.Error(t, err)

	_, err = drm.New(drm.Key, make([]byte, 3), khash)
	require.Error(t, err)

	_, err = drm.New(drm.Key, kid, nil)
	require.Error(t, err)

	_, err = drm.New(drm.Key, kid, &hash.Hash{Type: hash.Type{Code: hash.UNKNOWN, Format: hash.Unencrypted}, Digest: hdigest, ID: hid, Size: 1234})
	require.Error(t, err)

	_, err = drm.New(drm.Key, kid, &hash.Hash{Type: hash.Type{Code: hash.QPart, Format: hash.Unencrypted}, Digest: hdigest, Size: 1234})
	require.Error(t, err)
}

func TestString(t *testing.T) {
	kString := key.String()
	assert.Equal(t, keyString, kString)

	k, err := drm.FromString(keyString)
	assert.NoError(t, err)

	assert.Equal(t, key, k)
	assert.Equal(t, keyString, fmt.Sprint(k))
	assert.Equal(t, keyString, fmt.Sprintf("%v", k))
	assert.Equal(t, "blub"+keyString, fmt.Sprintf("blub%s", k))

	hid := id.NewID(id.Q, []byte{1, 2, 3, 4})
	hdigest := make([]byte, sha256.Size)
	rand.Read(hdigest)
	khash := &hash.Hash{Type: hash.Type{Code: hash.Q, Format: hash.Unencrypted}, Digest: hdigest, ID: hid, Size: 1234}
	kid := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	k2 := &drm.KeyID{Code: drm.Key, ID: kid, Hash: khash}
	k3, err := drm.FromString(k2.String())
	require.NoError(t, err)

	require.Equal(t, k2.Code, k3.Code)
	require.Equal(t, k2.ID, k3.ID)
	require.Equal(t, k2.Hash, k3.Hash)
	require.Equal(t, k2, k3)

	tests := []struct {
		key string
	}{
		{key: "blub"},
		{key: "drm_"},
		{key: "drm_1111"},
		{key: "drm_ "},
		{key: "drm_QmYtUc4iTCbbfVSDNKvtQqrfyezPPnFvE33wFmutw9PBB"},
	}
	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			k, err := drm.FromString(test.key)
			assert.Error(t, err)
			assert.Nil(t, k)
		})
	}
}

func Example() {
	fmt.Println("key", "string", keyString)

	// Convert a drm key string to a drm key object
	k, _ := drm.FromString(keyString)
	fmt.Println("key", "object", k)

	// Extract the data of the drm key object
	fmt.Println("key", "code", k.Code)
	fmt.Println("key", "id", k.ID)
	fmt.Println("key", "hash", k.Hash)

	// Convert the raw bytes to a drm key object
	k2, _ := drm.New(k.Code, k.ID, k.Hash)
	fmt.Println("key", "from data", k2)

	// Output:
	// key string drm_2qU32EeeHhVBxMF9vC8ABUsF5rmbfY1a6TvUGr3EYpYv1EvAc1FAM9tkACxiYmPoPxaEfbTNzHsAVh4TyKGJEFCd1gM
	// key object drm_2qU32EeeHhVBxMF9vC8ABUsF5rmbfY1a6TvUGr3EYpYv1EvAc1FAM9tkACxiYmPoPxaEfbTNzHsAVh4TyKGJEFCd1gM
	// key code drm_
	// key id [17 87 89 18 6 191 136 142 131 156 105 189 28 159 165 77]
	// key hash hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7
	// key from data drm_2qU32EeeHhVBxMF9vC8ABUsF5rmbfY1a6TvUGr3EYpYv1EvAc1FAM9tkACxiYmPoPxaEfbTNzHsAVh4TyKGJEFCd1gM
}

type Wrapper struct {
	Key *drm.KeyID
}

func TestMarshalUnmarshal(t *testing.T) {
	b, err := json.Marshal(key)
	assert.NoError(t, err)
	assert.Equal(t, "\""+keyString+"\"", string(b))

	var unmarshalled *drm.KeyID
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, key, unmarshalled)

	s := Wrapper{Key: key}
	b, err = json.Marshal(s)
	assert.NoError(t, err)
	assert.Contains(t, string(b), keyString)

	var unmarshalled2 Wrapper
	err = json.Unmarshal(b, &unmarshalled2)
	assert.NoError(t, err)
	assert.Equal(t, s, unmarshalled2)
}

func TestEqual(t *testing.T) {
	require.True(t, key.Equal(key))

	other, err := drm.FromString(key.String())
	require.NoError(t, err)
	require.True(t, key.Equal(other))
	require.True(t, other.Equal(key))

	require.False(t, key.Equal(nil))
	require.False(t, key.Equal(&drm.KeyID{}))

	var nilKey *drm.KeyID
	require.False(t, nilKey.Equal(key))
	require.True(t, nilKey.Equal(nil))
}

func TestAssertEqual(t *testing.T) {
	require.NoError(t, key.AssertEqual(key))

	other, err := drm.FromString(key.String())
	require.NoError(t, err)
	require.NoError(t, key.AssertEqual(other))
	require.NoError(t, other.AssertEqual(key))

	testAssertEqual := func(key *drm.KeyID, other *drm.KeyID, err error) {
		ae := key.AssertEqual(other)
		require.True(t, errors.Match(err, ae), ae)
	}

	// other, err = drm.New(drm.UNKNOWN, key.ID, key.Hash)
	// require.NoError(t, err)
	// testAssertEqual(key, other, errors.E().With("reason", "code differs"))

	other, err = drm.New(key.Code, make([]byte, 16), key.Hash)
	require.NoError(t, err)
	testAssertEqual(key, other, errors.E().With("reason", "id differs"))

	other, err = drm.New(key.Code, key.ID, &hash.Hash{Type: key.Hash.Type, Digest: key.Hash.Digest, Size: 0, ID: key.Hash.ID})
	require.NoError(t, err)
	testAssertEqual(key, other, errors.E().With("reason", "hash differs"))

	var nilKey *drm.KeyID
	require.Error(t, nilKey.AssertEqual(key))
	require.NoError(t, nilKey.AssertEqual(nil))
}

func TestMaxLength(t *testing.T) {
	hdigest := make([]byte, sha256.Size)
	rand.Read(hdigest)
	hidbytes := make([]byte, 20)
	rand.Read(hidbytes)
	hid := id.NewID(id.Q, hidbytes)
	khash := &hash.Hash{Type: hash.Type{Code: hash.Q, Format: hash.Unencrypted}, Digest: hdigest, ID: hid, Size: 1024 * 1024 * 128}
	kid := make([]byte, 16)
	rand.Read(kid)
	k, err := drm.New(drm.Key, kid, khash)
	require.NoError(t, err)
	require.GreaterOrEqual(t, 4+99, len(k.String()))
	fmt.Println(k.String())
}
