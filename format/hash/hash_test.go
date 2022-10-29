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

	"github.com/eluv-io/common-go/format/types"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"

	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
)

var hsh *hash.Hash

const hashString = "hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7"

func init() {
	htype := hash.Type{Code: hash.Q, Format: hash.Unencrypted}
	digest, _ := hex.DecodeString("9cbc07c3f991725836a3aa2a581ca2029198aa420b9d99bc0e131d9f3e2cbe47")
	size := int64(1024)
	idx, _ := id.FromString("iq__WxoChT9EZU2PRdTdNU7Ldf")

	hsh = &hash.Hash{Type: htype, Digest: digest, Size: size, ID: idx}
}

func TestEmptyHash(t *testing.T) {
	h := &hash.Hash{
		Type: hash.Type{
			Code: hash.Q,
		},
	}
	require.False(t, h.IsNil())
	// PENDING(GIL): shouldn't we have a IsValid() function ?
	require.Equal(t, "", h.String())
}

func TestHashCtorError(t *testing.T) {
	digest := make([]byte, sha256.Size)
	rand.Read(digest)
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
		hl1, err := hash.NewLive(
			hash.Type{Code: hash.QPartLive, Format: hash.Unencrypted},
			digest,
			utc.Now(),
		)
		require.NoError(t, err)
		hl2, err := hash.FromString(hl1.String())
		require.NoError(t, err)
		require.Equal(t, hl1, hl2)

		_, err = hash.NewLive(
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

	d := hash.NewDigest(sha256.New(), hash.Type{Code: hash.Q, Format: hash.Unencrypted}).WithID(idx)
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

	d := hash.NewDigest(sha256.New(), hash.Type{Code: hash.Q, Format: hash.Unencrypted}).WithID(idx)
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
}

func TestTQ(t *testing.T) {
	h, err := hash.FromString(hashString)
	require.NoError(t, err)

	htq, err := h.As(hash.TQ, types.NewTQID(h.ID, id.NewID(id.Tenant, []byte{99})).ID())
	require.NoError(t, err)

	require.Equal(t, h.Digest, htq.Digest)
	require.Equal(t, h.Size, htq.Size)
	require.Equal(t, h.Type.Format, htq.Type.Format)
	require.Equal(t, h.Expiration, htq.Expiration)
	require.Equal(t, h.PreambleSize, htq.PreambleSize)

	// re-create htq, but with "wrong" type hash.Q instead of hash.TQ
	htq2, err := h.As(hash.Q, types.NewTQID(h.ID, id.NewID(id.Tenant, []byte{99})).ID())
	// make sure type is corrected
	require.Equal(t, htq, htq2)

}

func TestIsCompatible(t *testing.T) {
	tests := []struct {
		c1         hash.Code
		c2         hash.Code
		compatible bool
	}{
		{hash.Q, hash.Q, true},
		{hash.Q, hash.TQ, true},
		{hash.QPart, hash.QPart, true},
		{hash.Q, hash.QPart, false},
		{hash.TQ, hash.QPart, false},
	}
	for _, test := range tests {
		t.Run(fmt.Sprint(test.c1, test.c2, test.compatible), func(t *testing.T) {
			require.Equal(t, test.compatible, test.c1.IsCompatible(test.c2))
			require.Equal(t, test.compatible, test.c2.IsCompatible(test.c1))
		})
	}
}

func ExampleHash_Describe() {

	h, _ := hash.FromString(hashString)
	fmt.Println(h.Describe())
	fmt.Println()

	ph, _ := h.As(hash.QPart, nil)
	fmt.Println(ph.Describe())
	fmt.Println()

	htq, _ := hash.FromString(hashString)
	htq, _ = htq.As(hash.TQ, types.NewTQID(htq.ID, id.NewID(id.Tenant, []byte{99})).ID())
	fmt.Println(htq.Describe())
	fmt.Println()

	hqpe, _ := hash.FromString("hqpedYvWGgmzmerRxa2Rzv6dqjDogfCZE7dwSuDnfgaSfGbMeXXnT")
	fmt.Println(hqpe.Describe())
	fmt.Println()

	hql, _ := hash.FromString("hql_7TmHLg49Qd4NtgfcPeWKAG7fsk7HujeMHaE33Bwm2kLYDdjqYJw")
	fmt.Println(hql.Describe())
	fmt.Println()

	hqt, _ := hash.FromString("hqt_2KhUoLeUJFR3pfBWpYzSqTWA3PoP6vBqEZGTLYu")
	fmt.Println(hqt.Describe())
	fmt.Println()

	// Output:
	//
	// hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7
	// type:          content (code 1), unencrypted
	// digest:        0x9cbc07c3f991725836a3aa2a581ca2029198aa420b9d99bc0e131d9f3e2cbe47
	// size:          1024
	// qid:           iq__WxoChT9EZU2PRdTdNU7Ldf content 0xf2a366bab61847e9bd10b4ac5ed27bba (16 bytes)
	// part:          hqp_4YWKwzD4cymG9DodGRLphDg8fi2euXRgyYq9euQkjZx4a39
	//                type:          content part (code 2), unencrypted
	//                digest:        0x9cbc07c3f991725836a3aa2a581ca2029198aa420b9d99bc0e131d9f3e2cbe47
	//                size:          1024
	//
	// hqp_4YWKwzD4cymG9DodGRLphDg8fi2euXRgyYq9euQkjZx4a39
	// type:          content part (code 2), unencrypted
	// digest:        0x9cbc07c3f991725836a3aa2a581ca2029198aa420b9d99bc0e131d9f3e2cbe47
	// size:          1024
	//
	// htq_3s4GEwC2Gjs4a9AZ4Tefp4TfAJ8ctV6gX8MsoZB4RvKF2uv8dHQJt3D8qd3vjB6wFNg3yAgpq
	// type:          tenant content (code 5), unencrypted
	// digest:        0x9cbc07c3f991725836a3aa2a581ca2029198aa420b9d99bc0e131d9f3e2cbe47
	// size:          1024
	// qid:           itq_4M1DHfv8Wu1A2QmKWJG1WydB content with embedded tenant 0x0163f2a366bab61847e9bd10b4ac5ed27bba (18 bytes)
	//                  primary : iq__WxoChT9EZU2PRdTdNU7Ldf content 0xf2a366bab61847e9bd10b4ac5ed27bba (16 bytes)
	//                  embedded: iten2i tenant 0x63 (1 bytes)
	// part:          hqp_4YWKwzD4cymG9DodGRLphDg8fi2euXRgyYq9euQkjZx4a39
	//                type:          content part (code 2), unencrypted
	//                digest:        0x9cbc07c3f991725836a3aa2a581ca2029198aa420b9d99bc0e131d9f3e2cbe47
	//                size:          1024
	//
	// hqpedYvWGgmzmerRxa2Rzv6dqjDogfCZE7dwSuDnfgaSfGbMeXXnT
	// type:          content part (code 2), encrypted with AES-128, AFGHG BLS12-381, 1 MB block size
	// digest:        0x52fdfc072182654f163f5f0f9a621d729566c74d10037c4d7bbb0407d1e2c649
	// size:          1234
	// preamble_size: 567
	//
	// hql_7TmHLg49Qd4NtgfcPeWKAG7fsk7HujeMHaE33Bwm2kLYDdjqYJw
	// type:          live part (code 3), unencrypted
	// digest:        0xaffc42f44e3f73204569984e909d89a2086f2aba19c3866b7dd8b1861451f78c
	// expiration:    2020-12-15T12:00:00.000Z
	//
	// hqt_2KhUoLeUJFR3pfBWpYzSqTWA3PoP6vBqEZGTLYu
	// type:          transient live part (code 4), unencrypted
	// digest:        0x6665bb007a6007781f4f4a0940a67dc4b0fadec2e6fa26
	// expiration:    1992-08-04T00:00:00.000Z
}
