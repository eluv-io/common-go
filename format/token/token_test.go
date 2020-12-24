package token_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/qluvio/content-fabric/format/id"
	"github.com/qluvio/content-fabric/format/token"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	qid = id.MustParse("iq__99d4kp14eSDEP7HWfjU4W6qmqDw")
	nid = id.MustParse("inod3Sa5p3czRyYi8GnVGnh8gBDLaqJr")
	tok = func() *token.Token {
		t := token.New(token.QWrite, qid, nid)
		t.Bytes = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		return t
	}()
)

const expTokenString = "tq__8UmhDD9cZah58THfAYPf3Shj9hVzfwT51Cf4ZHKpayajzZRyMwCPiSpfS5yqRZfjkDjrtXuRmDa"

func TestConversion(t *testing.T) {
	testConversion(t, tok, token.QWrite, "tq__")
	testConversion(t, token.Generate(token.QWriteV1), token.QWriteV1, "tqw_")
	testConversion(t, token.Generate(token.QPartWrite), token.QPartWrite, "tqpw")
}

func testConversion(t *testing.T, tok *token.Token, code token.Code, prefix string) {
	fmt.Println(tok.String())

	err := tok.AssertCode(code)
	require.NoError(t, err)

	encoded := tok.String()
	assert.Equal(t, prefix, encoded[:4])

	decoded, err := token.FromString(encoded)
	assert.NoError(t, err)
	err = decoded.AssertCode(code)
	require.NoError(t, err)

	assert.Equal(t, tok.String(), decoded.String())
	assert.True(t, tok.Equal(decoded))

	assert.Equal(t, encoded, fmt.Sprint(tok))
	assert.Equal(t, encoded, fmt.Sprintf("%v", tok))
	assert.Equal(t, "blub"+encoded, fmt.Sprintf("blub%s", tok))
}

func TestInvalidStringConversions(t *testing.T) {
	tests := []struct {
		tok string
	}{
		{tok: ""},
		{tok: "blub"},
		{tok: "tqw_"},
		{tok: "qwt_00001111"},
		{tok: "tqw "},
		{tok: "tqw_1W7LcTy70"},
		{tok: "tq__xevbBFoiALJxdwZdxpR5XBvfqvTaDxf7"}, // a tqw_ with the w removed...
	}
	for _, test := range tests {
		t.Run(test.tok, func(t *testing.T) {
			tok, err := token.FromString(test.tok)
			assert.Error(t, err)
			assert.Nil(t, tok)
			fmt.Println(err)
		})
	}
}

func TestInvalidStringConversions2(t *testing.T) {
	tests := []struct {
		tok string
	}{
		{tok: ""},
		{tok: "blub"},
		{tok: "qwt_"},
		{tok: "qwt_00001111"},
		{tok: "qwt "},
		{tok: "tqw_1W7LcTy70"},
		{tok: token.Generate(token.QPartWrite).String()},
	}
	for _, test := range tests {
		t.Run(test.tok, func(t *testing.T) {
			tok, err := token.QWriteV1.FromString(test.tok)
			assert.Error(t, err)
			assert.Nil(t, tok)
		})
	}
}

func TestJSON(t *testing.T) {
	b, err := json.Marshal(tok)
	assert.NoError(t, err)
	assert.Equal(t, "\""+expTokenString+"\"", string(b))

	var unmarshalled token.Token
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.True(t, tok.Equal(&unmarshalled))
	assert.Equal(t, tok.String(), unmarshalled.String())
}

type Wrapper struct {
	Token *token.Token
}

func TestWrappedJSON(t *testing.T) {
	s := Wrapper{
		Token: tok,
	}
	b, err := json.Marshal(s)
	assert.NoError(t, err)
	assert.Contains(t, string(b), expTokenString)

	fmt.Println(string(b))

	var unmarshalled Wrapper
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.True(t, s.Token.Equal(unmarshalled.Token))
}

func ExampleToken_Describe() {
	tok, _ := token.FromString("tq__3WhUFGKoJAzvqrDWiZtkcfQHiKp4Gda4KkiwuRgX6BTFfq7hNeji2hPDW6qZxLuk7xAju4bgm8iLwK")
	fmt.Println(tok.Describe())

	// Output:
	//
	// type:   content write token
	// qid:    iq__1Bhh3pU9gLXZiNDL6PEZuEP5ri
	// nid:    inod2KRn6vRvn8U3gczhSMJwd1
	// random: 0xe6ded2a798ac1f820fe871c6170b6d12

}
