package jsonutil

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/qluvio/content-fabric/errors"
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

func TestStringer(t *testing.T) {
	type someStruct struct {
		Name   string `json:"name,omitempty"`
		Number int    `json:"number,omitempty"`
	}
	structA := &someStruct{"A", 1}
	structB := &someStruct{"B", 2}

	tests := []struct {
		val  interface{}
		want string
	}{
		{"just a string", `"just a string"`},
		{99, `99`},
		{structA, `{"name":"A","number":1}`},
		{structB, `{"name":"B","number":2}`},
		{func() interface{} { return structA }, `{"name":"A","number":1}`},
		{func() interface{} { return structB }, `{"name":"B","number":2}`},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%#v", tt.val), func(t *testing.T) {
			assert.Equal(t, tt.want, Stringer(tt.val).String())

			m, err := json.Marshal(Stringer(tt.val))
			assert.NoError(t, err)
			assert.Equal(t, tt.want, string(m))
		})
	}

	assert.Equal(t, `&jsonutil.unmarshalable{name:"test"}`, Stringer(&unmarshalable{"test"}).String())
}

type unmarshalable struct {
	name string
}

func (u *unmarshalable) MarshalJSON() ([]byte, error) {
	return nil, errors.E("marshal", errors.K.NotImplemented)
}
