package duration

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSeconds_FromString(t *testing.T) {
	tests := []struct {
		have    string
		want    Seconds
		wantErr bool
	}{
		{"", 0, true},
		{"0.0", 0, false},
		{"1.0", Seconds(time.Second), false},
		{"-1.0", Seconds(-time.Second), false},
		{"23.159", Seconds(23*time.Second + 159*time.Millisecond), false},
		{"0.1", Seconds(100 * time.Millisecond), false},
	}
	for _, tt := range tests {
		t.Run(tt.have, func(t *testing.T) {
			t.Run("text", func(t *testing.T) {
				got, err := SecondsFromString(tt.have)
				if tt.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, tt.want, got)

					marshaled, err := got.MarshalText()
					require.NoError(t, err)
					require.Equal(t, tt.have, string(marshaled))
				}
			})
			t.Run("json", func(t *testing.T) {
				var got Seconds
				err := json.Unmarshal([]byte(tt.have), &got)
				if tt.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, tt.want, got)

					marshaled, err := json.Marshal(got)
					require.NoError(t, err)
					require.Equal(t, tt.have, string(marshaled))
				}
			})
		})
	}
}

func TestSeconds_UnmarshalJSON(t *testing.T) {
	want := Seconds(23*time.Second + 159*time.Millisecond)
	var got struct {
		S1 Seconds `json:"s1"`
		S2 Seconds `json:"s2"`
		S3 Seconds `json:"s3"`
	}
	err := json.Unmarshal([]byte(`{"s1":"23.159","s2":"23.159s","s3":23.159}`), &got)
	require.NoError(t, err)
	require.Equal(t, want, got.S1)
	require.Equal(t, want, got.S2)
	require.Equal(t, want, got.S3)

	marshaled, err := json.Marshal(got)
	require.NoError(t, err)
	require.Equal(t, `{"s1":23.159,"s2":23.159,"s3":23.159}`, string(marshaled))
}
