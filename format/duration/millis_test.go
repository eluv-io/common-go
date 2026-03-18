package duration

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMillis_FromString(t *testing.T) {
	tests := []struct {
		have    string
		want    Millis
		wantErr bool
	}{
		{"", 0, true},
		{"0.000", 0, false},
		{"1000.000", Millis(time.Second), false},
		{"-1000.000", Millis(-time.Second), false},
		{"23159.000", Millis(23*time.Second + 159*time.Millisecond), false},
		{"100.000", Millis(100 * time.Millisecond), false},
	}
	for _, tt := range tests {
		t.Run(tt.have, func(t *testing.T) {
			t.Run("text", func(t *testing.T) {
				got, err := MillisFromString(tt.have)
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
				var got Millis
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

func TestMillis_UnmarshalJSON(t *testing.T) {
	want := Millis(23*time.Second + 159*time.Millisecond + 563*time.Microsecond)
	var got struct {
		S1 Millis `json:"s1"`
		S2 Millis `json:"s2"`
		S3 Millis `json:"s3"`
	}
	err := json.Unmarshal([]byte(`{"s1":"23159.563","s2":"23.159563s","s3":23159.563}`), &got)
	require.NoError(t, err)
	require.Equal(t, want, got.S1)
	require.Equal(t, want, got.S2)
	require.Equal(t, want, got.S3)

	marshaled, err := json.Marshal(got)
	require.NoError(t, err)
	require.Equal(t, `{"s1":23159.563,"s2":23159.563,"s3":23159.563}`, string(marshaled))
}
