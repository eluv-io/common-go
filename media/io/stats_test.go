package io

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConnStats_JSONFieldNames locks in snake_case JSON field names for ConnStats/SrtConnStats - both are exported
// wholesale by callers (e.g. avpipe's ExportedStats.Srt) into dashboard-facing JSON, so a default Go field-name
// regression here would silently break every consumer's field lookups.
func TestConnStats_JSONFieldNames(t *testing.T) {
	stats := ConnStats{
		RemoteAddr: "1.2.3.4:5678",
		LocalAddr:  "0.0.0.0:9999",
		SRT:        &SrtConnStats{Version: 5, Encrypted: true},
	}

	bb, err := json.Marshal(stats)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(bb, &decoded))
	require.Equal(t, "1.2.3.4:5678", decoded["remote_addr"])
	require.Equal(t, "0.0.0.0:9999", decoded["local_addr"])

	srtFields, ok := decoded["srt"].(map[string]any)
	require.True(t, ok, "decoded JSON must have an \"srt\" object")
	require.EqualValues(t, 5, srtFields["version"])
	require.Equal(t, true, srtFields["encrypted"])
}

// TestConnStats_OmitsEmptyFields verifies RemoteAddr/LocalAddr/SRT are all omitted (not just empty/null) when unset,
// so a non-SRT or not-yet-connected caller's JSON stays minimal.
func TestConnStats_OmitsEmptyFields(t *testing.T) {
	bb, err := json.Marshal(ConnStats{})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(bb, &decoded))
	require.NotContains(t, decoded, "remote_addr")
	require.NotContains(t, decoded, "local_addr")
	require.NotContains(t, decoded, "srt")
}
