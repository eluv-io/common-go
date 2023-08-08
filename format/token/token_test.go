package token_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/codecs"
	"github.com/eluv-io/common-go/format/encryption"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/token"
	"github.com/eluv-io/common-go/format/types"
)

var (
	qid  = id.MustParse("iq__99d4kp14eSDEP7HWfjU4W6qmqDw")
	tqid = types.NewTQID(id.NewID(id.Q, []byte{1, 2, 3}), id.NewID(id.Tenant, []byte{99})).ID()
	nid  = id.MustParse("inod3Sa5p3czRyYi8GnVGnh8gBDLaqJr")
	qwt  = func() *token.Token {
		t, _ := token.NewObject(token.QWrite, qid, nid, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9)
		return t
	}()
	tqwt = func() *token.Token {
		t, _ := token.NewObject(token.QWrite, tqid, nid, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9)
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

const (
	expTokenString = "tqw__8UmhDD9cZah58THfAYPf3Shj9hVzfwT51Cf4ZHKpayajzZRyMwCPiSpfS5yqRZfjkDjrtXuRmDa"
	expTqwtString  = "tqw__JUzjYYRw7qNCmuvfjRvC4QwRsYS3To9CZjeGKZqcrDy4ZGdY4aTY9W"
)

func TestBackwardsCompatibilityHack(t *testing.T) {
	tok, err := token.Parse("tq__8UmhDD9cZah58THfAYPf3Shj9hVzfwT51Cf4ZHKpayajzZRyMwCPiSpfS5yqRZfjkDjrtXuRmDa")
	require.NoError(t, err)

	tokBackwardsCompat, err := token.Parse("tqw__8UmhDD9cZah58THfAYPf3Shj9hVzfwT51Cf4ZHKpayajzZRyMwCPiSpfS5yqRZfjkDjrtXuRmDa")
	require.NoError(t, err)

	require.True(t, tok.Equal(tokBackwardsCompat))
}

func TestConversion(t *testing.T) {
	testConversion(t, qwt, token.QWrite, "tqw__")
	testConversion(t, tqwt, token.QWrite, "tqw__")
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

func TestNil(t *testing.T) {
	tok := (*token.Token)(nil)
	require.Nil(t, tok)
	require.True(t, tok.IsNil())
	require.False(t, tok.IsValid())
	require.True(t, tok.Equal(nil))
	require.Contains(t, tok.AssertCode(token.QWrite).Error(), "token is nil")
	require.Equal(t, "nil", tok.Describe())
	require.Contains(t, tok.Validate().Error(), "token is nil")

	bts, err := json.Marshal(tok)
	require.NoError(t, err)
	require.Equal(t, `null`, string(bts))
	tokWrapper := struct {
		Token *token.Token `json:"token"`
	}{}
	err = json.Unmarshal([]byte(`{"token":null}`), &tokWrapper)
	require.NoError(t, err)
	require.True(t, tokWrapper.Token.IsNil())

	buf := bytes.NewBuffer(nil)
	err = codecs.CborMuxCodec.Encoder(buf).Encode(tokWrapper)
	require.NoError(t, err)
	err = codecs.CborMuxCodec.Decoder(buf).Decode(&tokWrapper)
	require.NoError(t, err)
	require.True(t, tokWrapper.Token.IsNil())
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
	tests := []struct {
		tok  *token.Token
		want string
	}{
		{
			tok:  qwt,
			want: expTokenString,
		},
		{
			tok:  tqwt,
			want: expTqwtString,
		},
	}
	for i, test := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			b, err := json.Marshal(test.tok)
			assert.NoError(t, err)
			assert.Equal(t, "\""+test.want+"\"", string(b))

			var unmarshalled token.Token
			err = json.Unmarshal(b, &unmarshalled)
			assert.NoError(t, err)
			assert.True(t, test.tok.Equal(&unmarshalled))
			assert.Equal(t, test.tok.String(), unmarshalled.String())
		})
	}
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

func ExampleToken_Describe_object() {
	tok, _ := token.FromString("tq__3WhUFGKoJAzvqrDWiZtkcfQHiKp4Gda4KkiwuRgX6BTFfq7hNeji2hPDW6qZxLuk7xAju4bgm8iLwK")
	fmt.Println(tok.Describe())

	// Note on the output: the token is formatted with tqw__ prefix (5 chars) instead of tq__ because we still use the
	// backwards compatibility hack explained in the comment for `qwPrefix` in token.go line 167.

	// Output:
	//
	// tqw__3WhUFGKoJAzvqrDWiZtkcfQHiKp4Gda4KkiwuRgX6BTFfq7hNeji2hPDW6qZxLuk7xAju4bgm8iLwK
	// type:    content write token
	// bytes:   0xe6ded2a798ac1f820fe871c6170b6d12
	// content: iq__1Bhh3pU9gLXZiNDL6PEZuEP5ri content 0x000102030405060708090a0b0c0d0e0f10111213 (20 bytes)
	// node:    inod2KRn6vRvn8U3gczhSMJwd1 fabric node 0x0aabcbd87f414c0197efef1f52b305c8 (16 bytes)
}

func ExampleToken_Describe_object_2() {
	tok, _ := token.FromString("tqw__JUzjYYRw7qNCmuvfjRvC4QwRsYS3To9CZjeGKZqcrDy4ZGdY4aTY9W")
	fmt.Println(tok.Describe())

	// Output:
	//
	// tqw__JUzjYYRw7qNCmuvfjRvC4QwRsYS3To9CZjeGKZqcrDy4ZGdY4aTY9W
	// type:    content write token
	// bytes:   0x00010203040506070809
	// content: itq_A5JwgE content with embedded tenant 0x0163010203 (5 bytes)
	//            primary : iq__Ldp content 0x010203 (3 bytes)
	//            embedded: iten2i tenant 0x63 (1 bytes)
	// node:    inod3Sa5p3czRyYi8GnVGnh8gBDLaqJr fabric node 0xaf33e7ed62938a0499453d419461ca9d598950a3 (20 bytes)
}

func ExampleToken_Describe_part() {
	tok, _ := token.FromString("tqp_NHG92YAkoUg7dnCrWT8J3RLp6")
	fmt.Println(tok.Describe())

	// Output:
	//
	// tqp_NHG92YAkoUg7dnCrWT8J3RLp6
	// type:    content part write token
	// bytes:   0x5b28b6f7c5410bff09967db0e7e1a997
	// scheme:  cgck
	// flags:   [preamble]
}

func ExampleToken_Describe_lro() {
	tok, _ := token.FromString("tlro12hb4zikV2ArEoXXyUV6xKJPfC6Ff2siNKDKBVM6js8adif81")
	fmt.Println(tok.Describe())

	// Output:
	//
	// tlro12hb4zikV2ArEoXXyUV6xKJPfC6Ff2siNKDKBVM6js8adif81
	// type:    bitcode LRO handle
	// bytes:   0x2df2a5d3d6c4e0830a95e7f1e8c779f6
	// node:    inod2KRn6vRvn8U3gczhSMJwd1 fabric node 0x0aabcbd87f414c0197efef1f52b305c8 (16 bytes)
}
