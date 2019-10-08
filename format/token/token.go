package token

import (
	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/log"
	"fmt"

	"crypto/rand"
	mrand "math/rand"

	"github.com/mr-tron/base58/base58"
)

// PENDING(LUK): Encode node info into Token.
// Also, do we need different flavors of tokens for qwrite, qread, etc.?

// Code is the type of a Token
type Code uint8

// FromString parses the given string and returns the Token. Returns an error
// if the string is not a Token or a Token of the wrong type.
func (c Code) FromString(s string) (Token, error) {
	id, err := FromString(s)
	if err != nil {
		n, ok := codeToName[c]
		if !ok {
			n = fmt.Sprintf("Unknown code %d", c)
		}
		return nil, errors.E("parse Token", err, "expected_type", n)
	}
	return id, id.AssertCode(c)
}

// lint off
const (
	Unknown Code = iota
	QWrite
	QRead
	QPartWrite
	QPartRead
)

const codeLen = 1
const prefixLen = 4

var codeToPrefix = map[Code]string{}
var prefixToCode = map[string]Code{
	"tunk": Unknown,
	"tqw_": QWrite,
	"tqr_": QRead,
	"tqpw": QPartWrite,
	"tqpr": QPartRead,
}
var codeToName = map[Code]string{
	Unknown:    "unknown",
	QWrite:     "content write",
	QRead:      "content read",
	QPartWrite: "content part write",
	QPartRead:  "content part read",
}

func init() {
	for prefix, code := range prefixToCode {
		if len(prefix) != prefixLen {
			log.Fatal("invalid Token prefix definition", "prefix", prefix)
		}
		codeToPrefix[code] = prefix
	}
}

// Token is the type representing a Token. Tokens follow the multiformat principle and
// are prefixed with their type (a varint). Unlike other multiformat
// implementations like multihash, the type is serialized to textual form
// (String(), JSON) as a short text prefix instead of their encoded varint for
// increased readability.
type Token []byte

func (t Token) String() string {
	if len(t) <= codeLen {
		return ""
	}
	return t.prefix() + base58.Encode(t[codeLen:])
}

// AssertCode checks whether the hash's code equals the provided code
func (t Token) AssertCode(c Code) error {
	if t.code() != c {
		return errors.E("token verify", errors.K.Invalid, "token code doesn't match",
			"expected", codeToPrefix[c],
			"actual", t.prefix())
	}
	return nil
}

func (t Token) IsNil() bool {
	return len(t) == 0
}

func (t Token) IsValid() bool {
	return len(t) > codeLen
}

func (t Token) prefix() string {
	p, found := codeToPrefix[t.code()]
	if !found {
		return codeToPrefix[Unknown]
	}
	return p
}

func (t Token) code() Code {
	return Code(t[0])
}

// MarshalText implements custom marshaling using the string representation.
func (t Token) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

// UnmarshalText implements custom unmarshaling from the string representation.
func (t *Token) UnmarshalText(text []byte) error {
	parsed, err := FromString(string(text))
	if err != nil {
		return errors.E("unmarshal token", errors.K.Invalid, err)
	}
	*t = parsed
	return nil
}

// Generate creates a random Token for the given Token type.
func Generate(code Code) Token {
	b := make([]byte, 24+codeLen)
	b[0] = byte(code)
	FillRand(b[1:])
	return Token(b)
}
func FillRand(b []byte) {
	read, err := rand.Read(b)
	if err != nil || read < len(b) {
		mrand.Read(b[read:])
	}
}

// FromString parses a Token from the given string representation.
func FromString(s string) (Token, error) {
	if len(s) <= prefixLen {
		return nil, errors.E("parse token", errors.K.Invalid).With("string", s)
	}

	code, found := prefixToCode[s[:prefixLen]]
	if !found {
		return nil, errors.E("parse token", errors.K.Invalid, "reason", "unknown prefix", "string", s)
	}

	dec, err := base58.Decode(s[prefixLen:])
	if err != nil {
		return nil, errors.E("parse token", errors.K.Invalid, err, "string", s)
	}
	b := []byte{byte(code)}
	return Token(append(b, dec...)), nil
}
