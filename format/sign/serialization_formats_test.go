package sign

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSerializationFormat_MarshalText(t *testing.T) {
	type A struct {
		Name string              `json:"name"`
		SF   SerializationFormat `json:"sf"`
	}

	type testCase struct {
		a       string
		wantSF  SerializationFormat
		wantErr bool
	}

	for i, tc := range []*testCase{
		{a: `{"name": "a", "sf": "scale"}`, wantSF: SerializationFormats.Scale()},
		{a: `{"name": "c", "sf": "sc"}`, wantSF: SerializationFormats.Unknown()},
		{a: `{"name": "c", "sf": "bla"}`, wantSF: SerializationFormats.Unknown()},
		{a: `{"name": "e", "sf": "eth_keccak"}`, wantSF: SerializationFormats.EthKeccak()},
		{a: `{"name": "e", "sf": "unknown"}`, wantSF: SerializationFormats.Unknown()},
	} {
		a := &A{}
		err := json.Unmarshal([]byte(tc.a), a)
		if tc.wantErr {
			require.Error(t, err, "case %d: %s", i, tc.a)
			continue
		}
		require.NoError(t, err, "case %d: %s", i, tc.a)
		require.True(t, tc.wantSF == a.SF)
		switch tc.wantSF {
		case SerializationFormats.Unknown():
			require.True(t, a.SF.Unknown())
		case SerializationFormats.Scale():
			require.True(t, a.SF.Scale())
		case SerializationFormats.EthKeccak():
			require.True(t, a.SF.EthKeccak())
		}
	}

}
