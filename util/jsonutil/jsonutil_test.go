package jsonutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsJson(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		partial bool
		want    bool
	}{
		{"empty", ``, false, true},

		{"json obj", `{"some":"prop"}`, false, true},
		{"json arr", `["one","two","three"]`, false, true},
		{"json string", `"some string"`, false, true},
		{"json int", `123456`, false, true},
		{"json float", `123456.78`, false, true},

		{"partial obj", `{"some":"pr`, true, true},
		{"partial json arr", `["one",`, true, true},
		{"partial json string", `"some `, true, true},

		{"invalid 1", `some `, false, false},
		{"invalid 2", string([]byte{0, 1, 2, 3, 4}), false, false},
		{"invalid 3", `<xml><prop bla="blub/></xml>"`, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsJson([]byte(tt.json), tt.partial))
			if !tt.want {
				// invalid JSON should be invalid regardless whether it's
				// partial or not...
				assert.Equal(t, tt.want, IsJson([]byte(tt.json), !tt.partial))
			}
			if tt.json == "" {
				// just a hack to test nil, since strings cannot be nil...
				assert.Equal(t, tt.want, IsJson(nil, tt.partial))
			}
		})
	}
}
