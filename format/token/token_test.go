package token_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/encryption"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/token"
)

var (
	qid = id.MustParse("iq__99d4kp14eSDEP7HWfjU4W6qmqDw")
	nid = id.MustParse("inod3Sa5p3czRyYi8GnVGnh8gBDLaqJr")
	qwt = func() *token.Token {
		t, _ := token.NewObject(token.QWrite, qid, nid, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9)
		return t
	}()
	qpwt = func() *token.Token {
		t, _ := token.NewPart(token.QPartWrite, encryption.ClientGen, token.PreambleQPWFlag, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9)
		return t
	}()
	lrot = func() *token.Token {
		t, _ := token.NewLRO(token.LRO, nid, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9)
		return t
	}()
)

const expTokenString = "tqw__8UmhDD9cZah58THfAYPf3Shj9hVzfwT51Cf4ZHKpayajzZRyMwCPiSpfS5yqRZfjkDjrtXuRmDa"

func TestBackwardsCompatibilityHack(t *testing.T) {
	tok, err := token.Parse("tq__8UmhDD9cZah58THfAYPf3Shj9hVzfwT51Cf4ZHKpayajzZRyMwCPiSpfS5yqRZfjkDjrtXuRmDa")
	require.NoError(t, err)

	tokBackwardsCompat, err := token.Parse("tq__8UmhDD9cZah58THfAYPf3Shj9hVzfwT51Cf4ZHKpayajzZRyMwCPiSpfS5yqRZfjkDjrtXuRmDa")
	require.NoError(t, err)

	require.Equal(t, tok, tokBackwardsCompat)
}

func TestConversion(t *testing.T) {
	testConversion(t, qwt, token.QWrite, "tqw__")
	testConversion(t, token.Generate(token.QWriteV1), token.QWriteV1, "tqw_")
	testConversion(t, qpwt, token.QPartWrite, "tqp_")
	testConversion(t, token.Generate(token.QPartWriteV1), token.QPartWriteV1, "tqpw")
	testConversion(t, lrot, token.LRO, "tlro")
}

func testConversion(t *testing.T, tok *token.Token, code token.Code, prefix string) {
	fmt.Println(tok.String())

	err := tok.AssertCode(code)
	require.NoError(t, err)

	encoded := tok.String()
	assert.Equal(t, prefix, encoded[:len(prefix)])

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
		{tok: "tq__xevbBFoiALJxdwZdxpR5XBvfqvTaDxf7"},  // a tqw_ with the w removed...
		{tok: "tqw__xevbBFoiALJxdwZdxpR5XBvfqvTaDxf7"}, // a new QWrite with invalid data
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
	b, err := json.Marshal(qwt)
	assert.NoError(t, err)
	assert.Equal(t, "\""+expTokenString+"\"", string(b))

	var unmarshalled token.Token
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.True(t, qwt.Equal(&unmarshalled))
	assert.Equal(t, qwt.String(), unmarshalled.String())
}

type Wrapper struct {
	Token *token.Token
}

func TestWrappedJSON(t *testing.T) {
	s := Wrapper{
		Token: qwt,
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

func ExampleToken_Describe_Object() {
	tok, _ := token.FromString("tq__3WhUFGKoJAzvqrDWiZtkcfQHiKp4Gda4KkiwuRgX6BTFfq7hNeji2hPDW6qZxLuk7xAju4bgm8iLwK")
	fmt.Println(tok.Describe())

	// Output:
	//
	// type:   content write token
	// bytes:  0xe6ded2a798ac1f820fe871c6170b6d12
	// qid:    iq__1Bhh3pU9gLXZiNDL6PEZuEP5ri
	// nid:    inod2KRn6vRvn8U3gczhSMJwd1
}

func ExampleToken_Describe_Part() {
	tok, _ := token.FromString("tqp_NHG92YAkoUg7dnCrWT8J3RLp6")
	fmt.Println(tok.Describe())

	// Output:
	//
	// type:   content part write token
	// bytes:  0x5b28b6f7c5410bff09967db0e7e1a997
	// scheme: cgck
	// flags:  [preamble]
}

func ExampleToken_Describe_LRO() {
	tok, _ := token.FromString("tlro12hb4zikV2ArEoXXyUV6xKJPfC6Ff2siNKDKBVM6js8adif81")
	fmt.Println(tok.Describe())

	// Output:
	//
	// type:   bitcode LRO handle
	// bytes:  0x2df2a5d3d6c4e0830a95e7f1e8c779f6
	// nid:    inod2KRn6vRvn8U3gczhSMJwd1
}
