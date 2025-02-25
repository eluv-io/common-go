package hash_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/byteutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"

	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
)

var hsh *hash.Hash
var qid id.ID

const hashString = "hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7"

func init() {
	htype := hash.Type{Code: hash.Q, Format: hash.Unencrypted}
	digest, _ := hex.DecodeString("9cbc07c3f991725836a3aa2a581ca2029198aa420b9d99bc0e131d9f3e2cbe47")
	size := int64(1024)
	qid, _ = id.FromString("iq__WxoChT9EZU2PRdTdNU7Ldf")

	hsh = &hash.Hash{Type: htype, Digest: digest, Size: size, ID: qid}
}

func TestEmptyHash(t *testing.T) {
	h := &hash.Hash{
		Type: hash.Type{
			Code: hash.Q,
		},
	}
	require.False(t, h.IsNil())
	require.Error(t, h.Validate())
	fmt.Println(h.Validate())
}

func TestHashCtorError(t *testing.T) {
	digest := byteutil.RandomBytes(sha256.Size)
	idx := id.NewID(id.Q, []byte{1, 2, 3, 4})

	{ // Object hash
		h1, err := hash.NewObject(
			hash.Type{Code: hash.Q, Format: hash.Unencrypted},
			digest,
			1234,
			idx)
		require.NoError(t, err)
		h2, err := hash.FromString(h1.String())
		require.NoError(t, err)
		require.Equal(t, h1, h2)

		_, err = hash.NewObject(
			hash.Type{Code: hash.QPart, Format: hash.Unencrypted},
			digest,
			1234,
			idx)
		require.Error(t, err)

		_, err = hash.NewObject(
			hash.Type{Code: hash.Q, Format: hash.AES128AFGH},
			make([]byte, 3),
			1234,
			idx)
		require.Error(t, err)

		_, err = hash.NewObject(
			hash.Type{Code: hash.Q, Format: hash.Unencrypted},
			digest,
			-1234,
			idx)
		require.Error(t, err)

		_, err = hash.NewObject(
			hash.Type{Code: hash.Q, Format: hash.Unencrypted},
			digest,
			1234,
			nil)
		require.Error(t, err)

		_, err = hash.NewObject(
			hash.Type{Code: hash.Q, Format: hash.AES128AFGH},
			digest,
			1234,
			id.NewID(id.QLib, []byte{1, 2, 3, 4}))
		require.Error(t, err)
	}

	{ // Part hash
		hp1, err := hash.NewPart(
			hash.Type{Code: hash.QPart, Format: hash.Unencrypted},
			digest,
			1234,
			567,
		)
		require.NoError(t, err)
		hp2, err := hash.FromString(hp1.String())
		require.NoError(t, err)
		require.Equal(t, hp1, hp2)

		_, err = hash.NewPart(
			hash.Type{Code: hash.QPartLive, Format: hash.Unencrypted},
			digest,
			1234,
			0)
		require.Error(t, err)

		_, err = hash.NewPart(
			hash.Type{Code: hash.QPart, Format: hash.AES128AFGH},
			make([]byte, 3),
			1234,
			0)
		require.Error(t, err)

		_, err = hash.NewPart(
			hash.Type{Code: hash.QPart, Format: hash.Unencrypted},
			digest,
			-1234,
			0)
		require.Error(t, err)

		_, err = hash.NewPart(
			hash.Type{Code: hash.QPart, Format: hash.Unencrypted},
			digest,
			1234,
			-567)
		require.Error(t, err)
	}

	{ // Live hash
		{
			hl1, err := hash.NewLive(
				hash.Type{Code: hash.QPartLive, Format: hash.Unencrypted},
				digest,
				utc.Now(),
			)
			require.NoError(t, err)
			hl2, err := hash.FromString(hl1.String())
			require.NoError(t, err)
			require.Equal(t, hl1, hl2)
		}

		{ // live hashes don't require a sha256 digest (can be of size != 32 bytes)
			hl1, err := hash.NewLive(
				hash.Type{Code: hash.QPartLive, Format: hash.Unencrypted},
				byteutil.RandomBytes(10),
				utc.Now(),
			)
			require.NoError(t, err)
			hl2, err := hash.FromString(hl1.String())
			require.NoError(t, err)
			require.Equal(t, hl1, hl2)
		}

		_, err := hash.NewLive(
			hash.Type{Code: hash.QPart, Format: hash.Unencrypted},
			digest,
			utc.Now())
		require.Error(t, err)
	}
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

func ExampleHash_String() {
	{ // Object hash
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
		h2, _ := hash.NewObject(h.Type, h.Digest, h.Size, h.ID)
		fmt.Println("hash", "from data", h2)
	}

	fmt.Println("")

	{ // Part hash
		hashString := "hqpedYvWGgmzmerRxa2Rzv6dqjDogfCZE7dwSuDnfgaSfGbMeXXnT"
		fmt.Println("hash", "string", hashString)

		// Convert a hash string to a hash object
		h, _ := hash.FromString(hashString)
		fmt.Println("hash", "object", h)

		// Extract the data of the hash object
		fmt.Println("hash", "type", h.Type)
		fmt.Println("hash", "digest", h.Digest)
		fmt.Println("hash", "size", h.Size)
		fmt.Println("hash", "preamble_size", h.PreambleSize)

		// Convert the raw bytes to a hash object
		h2, _ := hash.NewPart(h.Type, h.Digest, h.Size, h.PreambleSize)
		fmt.Println("hash", "from data", h2)
	}

	fmt.Println("")

	{ // Live hash
		hashString := "hql_7TmHLg49Qd4NtgfcPeWKAG7fsk7HujeMHaE33Bwm2kLYDdjqYJw"
		fmt.Println("hash", "string", hashString)

		// Convert a hash string to a hash object
		h, _ := hash.FromString(hashString)
		fmt.Println("hash", "object", h)

		// Extract the data of the hash object
		fmt.Println("hash", "type", h.Type)
		fmt.Println("hash", "digest", h.Digest)
		fmt.Println("hash", "expiration", h.Expiration)

		// Convert the raw bytes to a hash object
		h2, _ := hash.NewLive(h.Type, h.Digest, h.Expiration)
		fmt.Println("hash", "from data", h2)
	}

	// Output:
	// hash string hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7
	// hash object hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7
	// hash type hq__
	// hash digest [156 188 7 195 249 145 114 88 54 163 170 42 88 28 162 2 145 152 170 66 11 157 153 188 14 19 29 159 62 44 190 71]
	// hash size 1024
	// hash id iq__WxoChT9EZU2PRdTdNU7Ldf
	// hash from data hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7
	//
	// hash string hqpedYvWGgmzmerRxa2Rzv6dqjDogfCZE7dwSuDnfgaSfGbMeXXnT
	// hash object hqpedYvWGgmzmerRxa2Rzv6dqjDogfCZE7dwSuDnfgaSfGbMeXXnT
	// hash type hqpe
	// hash digest [82 253 252 7 33 130 101 79 22 63 95 15 154 98 29 114 149 102 199 77 16 3 124 77 123 187 4 7 209 226 198 73]
	// hash size 1234
	// hash preamble_size 567
	// hash from data hqpedYvWGgmzmerRxa2Rzv6dqjDogfCZE7dwSuDnfgaSfGbMeXXnT
	//
	// hash string hql_7TmHLg49Qd4NtgfcPeWKAG7fsk7HujeMHaE33Bwm2kLYDdjqYJw
	// hash object hql_7TmHLg49Qd4NtgfcPeWKAG7fsk7HujeMHaE33Bwm2kLYDdjqYJw
	// hash type hql_
	// hash digest [175 252 66 244 78 63 115 32 69 105 152 78 144 157 137 162 8 111 42 186 25 195 134 107 125 216 177 134 20 81 247 140]
	// hash expiration 2020-12-15T12:00:00.000Z
	// hash from data hql_7TmHLg49Qd4NtgfcPeWKAG7fsk7HujeMHaE33Bwm2kLYDdjqYJw
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
	idObj, _ := id.FromString("iq__WxoChT9EZU2PRdTdNU7Ldf")
	h, err := hash.NewObject(hash.Type{hash.Q, hash.Unencrypted}, digest, 1024, idObj)
	assert.NoError(t, err)
	assertHash(t, h, "hq_")
	// assertHash(t, GenerateAccountHash(), "acc")
	// assertHash(t, GenerateUserHash(), "usr")
	// assertHash(t, GenerateQLibHash(), "lib")
	// assertHash(t, GenerateQHash(), "q__")
}

func assertHash(t *testing.T, hsh *hash.Hash, expectedPrefix string) {
	fmt.Println(hsh)
	assert.NotNil(t, hsh)
	assert.Contains(t, hsh.String(), expectedPrefix)
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

	{
		hsid0, _ := hash.NewBuilder().BuildHash()
		hsid1, _ := hash.NewBuilder().WithStorageId(1).BuildHash()
		hsid2, _ := hash.NewBuilder().WithStorageId(2).BuildHash()
		require.True(t, hsid0.Equal(hsid0))
		require.True(t, hsid1.Equal(hsid1))
		require.True(t, hsid2.Equal(hsid2))
		require.False(t, hsid0.Equal(hsid1))
		require.False(t, hsid0.Equal(hsid2))

	}
}

func TestAssertEqual(t *testing.T) {
	require.NoError(t, hsh.AssertEqual(hsh))

	other, err := hash.FromString(hsh.String())
	require.NoError(t, err)
	require.NoError(t, hsh.AssertEqual(other))
	require.NoError(t, other.AssertEqual(hsh))

	testAssertEqual := func(hsh *hash.Hash, other *hash.Hash, err error) {
		ae := hsh.AssertEqual(other)
		fmt.Println(ae)
		require.True(t, errors.Match(err, ae), ae)
	}

	other, err = hash.NewPart(hash.Type{Code: hash.QPart, Format: hash.AES128AFGH}, hsh.Digest, hsh.Size, 0)
	require.NoError(t, err)
	testAssertEqual(hsh, other, errors.E().With("reason", "type differs"))

	dig := make([]byte, sha256.Size)
	other, err = hash.NewObject(hsh.Type, dig, hsh.Size, hsh.ID)
	require.NoError(t, err)
	testAssertEqual(hsh, other, errors.E().With("reason", "digest differs"))

	other, err = hash.NewObject(hsh.Type, hsh.Digest, hsh.Size+10, hsh.ID)
	require.NoError(t, err)
	testAssertEqual(hsh, other, errors.E().With("reason", "size differs"))

	id2, _ := id.FromString("iq__1Bhh3pU9gLXZiNDL6PEZuEP5ri")
	other, _ = hash.NewObject(hsh.Type, hsh.Digest, hsh.Size, id2)
	testAssertEqual(hsh, other, errors.E().With("reason", "id differs"))

	hsh2, _ := hash.NewPart(hash.Type{Code: hash.QPart, Format: hash.Unencrypted}, hsh.Digest, hsh.Size, 1234)
	other, _ = hash.NewPart(hash.Type{Code: hash.QPart, Format: hash.Unencrypted}, hsh.Digest, hsh.Size, 0)
	testAssertEqual(hsh2, other, errors.E().With("reason", "preamble size differs"))

	hsh2, _ = hash.NewLive(hash.Type{Code: hash.QPartLive, Format: hash.Unencrypted}, hsh.Digest, utc.Now())
	other, _ = hash.NewLive(hash.Type{Code: hash.QPartLive, Format: hash.Unencrypted}, hsh.Digest, utc.Now().Add(time.Minute))
	testAssertEqual(hsh2, other, errors.E().With("reason", "expiration differs"))

	var nilHash *hash.Hash
	require.Error(t, nilHash.AssertEqual(hsh))
	require.NoError(t, nilHash.AssertEqual(nil))

	{
		hsid0, _ := hash.NewBuilder().BuildHash()
		hsid1, _ := hash.NewBuilder().WithStorageId(1).BuildHash()
		hsid2, _ := hash.NewBuilder().WithStorageId(2).BuildHash()
		require.NoError(t, hsid0.AssertEqual(hsid0))
		require.NoError(t, hsid1.AssertEqual(hsid1))
		require.NoError(t, hsid2.AssertEqual(hsid2))
		testAssertEqual(hsid0, hsid1, errors.E().With("reason", "type differs"))
		testAssertEqual(hsid1, hsid2, errors.E().With("reason", "storage id differs"))
	}
}

// PENDING: remove after old live parts are finally deleted
func TestOldLivePart(t *testing.T) {
	oldLivePart := "hql_Kaxnnu3M3fYT6HA2zkGE4qCWzmRkNECkz"
	oldLivePart2 := "hql_7TmHLg49Qd4NtgfcPeWKAG7fsk7HujeMH"

	h, err := hash.FromString(oldLivePart)
	require.NoError(t, err)
	require.Equal(t, hash.QPartLive, h.Type.Code)
	require.Equal(t, hash.Unencrypted, h.Type.Format)
	require.Equal(t, 24, len(h.Digest))
	require.True(t, h.Expiration.IsZero())
	require.True(t, h.IsLive())

	h = &hash.Hash{Type: hash.Type{hash.QPartLive, hash.Unencrypted}, Digest: h.Digest}
	require.Equal(t, oldLivePart, h.String())

	h, err = hash.FromString(oldLivePart2)
	require.NoError(t, err)
	require.Equal(t, hash.QPartLive, h.Type.Code)
	require.Equal(t, hash.Unencrypted, h.Type.Format)
	require.Equal(t, 24, len(h.Digest))
	require.True(t, h.Expiration.IsZero())
	require.True(t, h.IsLive())

	h = &hash.Hash{Type: hash.Type{hash.QPartLive, hash.Unencrypted}, Digest: h.Digest}
	require.Equal(t, oldLivePart2, h.String())

	h, err = hash.NewLive(hash.Type{hash.QPartLive, hash.Unencrypted}, h.Digest, utc.Zero)
	require.Error(t, err)
	require.Nil(t, h)
}

func ExampleHash_Describe() {

	example := func(h *hash.Hash, err error) *hash.Hash {
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(h.String())
		fmt.Println(h.Describe())

		h2, err := hash.FromString(h.String())
		if err != nil {
			fmt.Println(err)
		}
		if h.String() != h2.String() {
			fmt.Println("round trip failed for", h)
		}
		return h
	}

	h := example(hash.FromString(hashString))
	_ = example(h.AsPartHash())
	_ = example(hash.FromString("hqpedYvWGgmzmerRxa2Rzv6dqjDogfCZE7dwSuDnfgaSfGbMeXXnT"))
	_ = example(hash.FromString("hql_7TmHLg49Qd4NtgfcPeWKAG7fsk7HujeMHaE33Bwm2kLYDdjqYJw"))
	_ = example(hash.FromString("hqt_2KhUoLeUJFR3pfBWpYzSqTWA3PoP6vBqEZGTLYu"))

	digest := hash.NewBuilder().WithStorageId(1)
	_, _ = digest.Write([]byte{0, 1, 2, 3, 4, 5})
	hcq := example(digest.BuildHash())
	_ = example(hcq.AsContentHash(h.ID))

	// Output:
	//
	// hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7
	// type:          content, unencrypted
	// storage id:    0 (default)
	// digest:        0x9cbc07c3f991725836a3aa2a581ca2029198aa420b9d99bc0e131d9f3e2cbe47
	// size:          1024
	// qid:           iq__WxoChT9EZU2PRdTdNU7Ldf
	// part:          hqp_4YWKwzD4cymG9DodGRLphDg8fi2euXRgyYq9euQkjZx4a39
	//
	// hqp_4YWKwzD4cymG9DodGRLphDg8fi2euXRgyYq9euQkjZx4a39
	// type:          content part, unencrypted
	// storage id:    0 (default)
	// digest:        0x9cbc07c3f991725836a3aa2a581ca2029198aa420b9d99bc0e131d9f3e2cbe47
	// size:          1024
	//
	// hqpedYvWGgmzmerRxa2Rzv6dqjDogfCZE7dwSuDnfgaSfGbMeXXnT
	// type:          content part, encrypted with AES-128, AFGHG BLS12-381, 1 MB block size
	// storage id:    0 (default)
	// digest:        0x52fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649
	// size:          1234
	// preamble_size: 567
	//
	// hql_7TmHLg49Qd4NtgfcPeWKAG7fsk7HujeMHaE33Bwm2kLYDdjqYJw
	// type:          live content part, unencrypted
	// storage id:    0 (default)
	// digest:        0xaffc42f44e3f73204569984e909d89a2086f2aba19c3866b7dd8b1861451f78c
	// expiration:    2020-12-15T12:00:00.000Z
	//
	// hqt_2KhUoLeUJFR3pfBWpYzSqTWA3PoP6vBqEZGTLYu
	// type:          transient live content part, unencrypted
	// storage id:    0 (default)
	// digest:        0x6665bb007a6007781f4f4a0940a67dc4b0fadec2e6fa26
	// expiration:    1992-08-04T00:00:00.000Z
	//
	// hcp_2S9iAJhixB6c524osbK6kD9dwAuCvWqgJiAKYP55EsCRCq
	// type:          content part with storage id, unencrypted
	// storage id:    1
	// digest:        0x17e88db187afd62c16e5debf3e6527cd006bc012bc90b51a810cd80c2d511f43
	// size:          6
	//
	// hc__nKYMsaPwjLWXyJqbvZMaR3zwSkpBNAYGm6nAVVvGWo5geY6NeAehC42Mq4CvBvwA8NM
	// type:          content with storage id, unencrypted
	// storage id:    1
	// digest:        0x17e88db187afd62c16e5debf3e6527cd006bc012bc90b51a810cd80c2d511f43
	// size:          6
	// qid:           iq__WxoChT9EZU2PRdTdNU7Ldf
	// part:          hcp_2S9iAJhixB6c524osbK6kD9dwAuCvWqgJiAKYP55EsCRCq
}

func TestHash_AsContentHash(t *testing.T) {
	toHash := func(d *hash.Digest) *hash.Hash {
		h, err := d.BuildHash()
		require.NoError(t, err)
		return h
	}
	requireError := func(h *hash.Hash, err error) {
		require.Error(t, err)
		require.Nil(t, h)
		fmt.Println(err)
	}

	t.Run("no conversion for encrypted part", func(t *testing.T) {
		h := toHash(hash.NewBuilder().WithFormat(hash.AES128AFGH))
		requireError(h.AsContentHash(qid))
	})

	t.Run("no conversion with encryption", func(t *testing.T) {
		h := toHash(hash.NewBuilder().WithPreamble(5))
		requireError(h.AsContentHash(qid))
	})

	t.Run("no conversion for live part", func(t *testing.T) {
		h, err := hash.NewLive(hash.Type{Code: hash.QPartLive, Format: hash.Unencrypted}, byteutil.RandomBytes(sha256.Size), utc.Now())
		require.NoError(t, err)
		requireError(h.AsContentHash(qid))
	})

	t.Run("no conversion for transient live part", func(t *testing.T) {
		h, err := hash.NewLive(hash.Type{Code: hash.QPartLiveTransient, Format: hash.Unencrypted}, byteutil.RandomBytes(sha256.Size), utc.Now())
		require.NoError(t, err)
		requireError(h.AsContentHash(qid))
	})

	t.Run("conversion for part hash", func(t *testing.T) {
		h := toHash(hash.NewBuilder())
		h, err := h.AsContentHash(qid)
		require.NoError(t, err)

		t.Run("replace ID", func(t *testing.T) {
			qid2 := id.MustParse("iq__1Bhh3pU9gLXZiNDL6PEZuEP5ri")
			h2, err := h.AsContentHash(qid2)
			require.NoError(t, err)
			require.Equal(t, qid2, h2.ID)
			require.NotEqual(t, h.String(), h2.String())
		})
	})

	t.Run("conversion for part with storage ID", func(t *testing.T) {
		h := toHash(hash.NewBuilder().WithStorageId(3))
		h, err := h.AsContentHash(qid)
		require.NoError(t, err)
		require.Equal(t, hash.C, h.Type.Code)

		t.Run("replace ID", func(t *testing.T) {
			qid2 := id.MustParse("iq__1Bhh3pU9gLXZiNDL6PEZuEP5ri")
			h2, err := h.AsContentHash(qid2)
			require.NoError(t, err)
			require.Equal(t, qid2, h2.ID)
			require.NotEqual(t, h.String(), h2.String())
			require.Equal(t, hash.C, h2.Type.Code)
		})
	})

}

func TestHash_AsPartHash(t *testing.T) {
	toHash := func(d *hash.Digest) *hash.Hash {
		_, _ = d.Write(byteutil.RandomBytes(99))
		h, err := d.BuildHash()
		require.NoError(t, err)
		return h
	}
	convert := func(h *hash.Hash) {
		ch, err := h.AsContentHash(qid)
		require.NoError(t, err)
		require.NotNil(t, ch)

		fmt.Println(ch.Describe())

		ph, err := ch.AsPartHash()
		require.NoError(t, err)
		require.Equal(t, h.String(), ph.String())
	}

	t.Run("part without storage ID", func(t *testing.T) {
		h := toHash(hash.NewBuilder())
		convert(h)
	})

	t.Run("part with storage ID", func(t *testing.T) {
		h := toHash(hash.NewBuilder().WithStorageId(3))
		convert(h)
	})

	t.Run("no conversion for part hash", func(t *testing.T) {
		h := toHash(hash.NewBuilder().WithStorageId(3))
		h2, err := h.AsPartHash()
		require.Error(t, err)
		require.Nil(t, h2)
	})

}
