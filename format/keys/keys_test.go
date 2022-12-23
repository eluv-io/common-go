package keys

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/byteutil"
	"github.com/eluv-io/errors-go"
)

func TestStringConversion(t *testing.T) {
	tests := []struct {
		code Code
		len  int
	}{
		{Primary, 9},
		{ES256KPublicKey, 33},
		{ED25519PublicKey, 32},
		{SR25519PublicKey, 32},
		{BLS12381PublicKey, 48},
	}
	for _, test := range tests {
		t.Run(fmt.Sprint(test.code), func(t *testing.T) {
			bts := byteutil.RandomBytes(test.len)
			key := New(test.code, bts)
			require.True(t, key.IsValid())

			fmt.Println(key)

			keyString := key.String()
			parsed, err := Parse(keyString)
			require.NoError(t, err)
			require.Equal(t, bts, parsed.Bytes())
			require.True(t, parsed.IsValid())
		})
	}
}

func TestInvalidStringConversions(t *testing.T) {
	tests := []struct {
		key    string
		reason string
	}{
		// {key: ""}, ==> returns nil, nil (no error, but nil key)
		{"blub", "empty key"},
		{"blub123", "unknown prefix"},
		{"kpsr", "empty key"},
		{"kpsr12345", "invalid key length"},
		{"kpsr111OO00", "invalid encoding"},
		{"kpsr ", "invalid encoding"},
	}
	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			key, err := FromString(test.key)
			assert.Error(t, err)
			assert.Nil(t, key)
			fmt.Println(err)
			reason, _ := errors.GetField(err, "reason")
			assert.Equal(t, test.reason, reason)
		})
	}
}

func TestJSON(t *testing.T) {
	key := New(ED25519PublicKey, byteutil.RandomBytes(32))
	b, err := json.Marshal(key)
	assert.NoError(t, err)
	assert.Equal(t, "\""+key.String()+"\"", string(b))

	var unmarshalled Key
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, key, unmarshalled)
}

type Wrapper struct {
	Key Key `json:"key"`
}

func TestWrappedJSON(t *testing.T) {
	key := New(ED25519PublicKey, byteutil.RandomBytes(32))
	s := Wrapper{
		Key: key,
	}
	b, err := json.Marshal(s)
	assert.NoError(t, err)
	assert.Contains(t, string(b), key.String())

	fmt.Println(string(b))

	var unmarshalled Wrapper
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, s, unmarshalled)
}
