package token_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/qluvio/content-fabric/format/token"

	"github.com/stretchr/testify/assert"
)

var tid = token.Token(append([]byte{1}, []byte{0, 1, 2, 3, 4, 5, 6}...))

const expTokenString = "tqw_1W7LcTy7"

func TestGenerate(t *testing.T) {
	generated := token.Generate(token.QWrite)
	fmt.Println(generated.String())

	generated.AssertCode(token.QWrite)

	idString := generated.String()
	assert.Equal(t, "tqw_", idString[:4])

	idFromString, err := token.FromString(idString)
	assert.NoError(t, err)
	idFromString.AssertCode(token.QWrite)

	assert.Equal(t, generated, idFromString)
}

func TestStringConversion(t *testing.T) {
	idString := tid.String()
	assert.Equal(t, expTokenString, idString)

	idFromString, err := token.FromString(idString)
	assert.NoError(t, err)
	idFromString.AssertCode(token.QWrite)

	assert.Equal(t, tid, idFromString)
	assert.Equal(t, idString, fmt.Sprint(tid))
	assert.Equal(t, idString, fmt.Sprintf("%v", tid))
	assert.Equal(t, "blub"+idString, fmt.Sprintf("blub%s", tid))
}

func TestInvalidStringConversions(t *testing.T) {
	tests := []struct {
		tok string
	}{
		{tok: ""},
		{tok: "blub"},
		{tok: "qwt_"},
		{tok: "qwt_00001111"},
		{tok: "qwt "},
		{tok: "tqw_1W7LcTy70"},
	}
	for _, test := range tests {
		t.Run(test.tok, func(t *testing.T) {
			tok, err := token.FromString(test.tok)
			assert.Error(t, err)
			assert.Nil(t, tok)
		})
	}
}

func TestJSON(t *testing.T) {
	b, err := json.Marshal(tid)
	assert.NoError(t, err)
	assert.Equal(t, "\""+expTokenString+"\"", string(b))

	var unmarshalled token.Token
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, tid, unmarshalled)
}

type Wrapper struct {
	Token token.Token
}

func TestWrappedJSON(t *testing.T) {
	s := Wrapper{
		Token: tid,
	}
	b, err := json.Marshal(s)
	assert.NoError(t, err)
	assert.Contains(t, string(b), expTokenString)

	fmt.Println(string(b))

	var unmarshalled Wrapper
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, s, unmarshalled)
}
