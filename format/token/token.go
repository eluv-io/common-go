package token

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/id"
	ei "github.com/qluvio/content-fabric/format/id"
	"github.com/qluvio/content-fabric/log"
	"github.com/qluvio/content-fabric/util/byteutil"

	"github.com/mr-tron/base58/base58"
)

func New(code Code, qid id.ID, nid id.ID) *Token {
	//if code == QWrite && (len(qid) == 0 || len(nid) == 0) {
	//	panic("qid and nid must not be nil!")
	//}
	return &Token{
		Code:  code,
		Bytes: byteutil.RandomBytes(16),
		QID:   qid,
		NID:   nid,
	}
}

// Code is the type of a Token
type Code uint8

// FromString parses the given string and returns the Token. Returns an error
// if the string is not a Token or a Token of the wrong type.
func (c Code) FromString(s string) (*Token, error) {
	token, err := FromString(s)
	if err != nil {
		n, ok := codeToName[c]
		if !ok {
			n = fmt.Sprintf("Unknown code %d", c)
		}
		return nil, errors.E("parse Token", err, "expected_type", n)
	}
	err = token.AssertCode(c)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (c Code) Describe() string {
	return codeToName[c]
}

// lint off
const (
	Unknown    Code = iota
	QWriteV1        // 1st version: just random bytes
	QWrite          // 2nd version: content ID, node ID, random bytes
	QPartWrite      // random bytes
)

const codeLen = 1
const prefixLen = 4

var codeToPrefix = map[Code]string{}
var prefixToCode = map[string]Code{
	"tunk": Unknown,
	"tqw_": QWriteV1,
	"tq__": QWrite,
	"tqpw": QPartWrite,
}
var codeToName = map[Code]string{
	Unknown:    "unknown",
	QWriteV1:   "content write token v1",
	QWrite:     "content write token",
	QPartWrite: "content part write token",
}

func init() {
	for prefix, code := range prefixToCode {
		if len(prefix) != prefixLen {
			log.Fatal("invalid Token prefix definition", "prefix", prefix)
		}
		codeToPrefix[code] = prefix
	}
}

// Token is the type representing a Token. Tokens follow the multiformat
// principle and are prefixed with their type (a varint). Unlike other
// multiformat implementations like multihash, the type is serialized to textual
// form (String(), JSON) as a short text prefix instead of their encoded varint
// for increased readability.
type Token struct {
	Code  Code
	Bytes []byte
	QID   ei.ID // content ID
	NID   ei.ID // node ID of insertion point node
	s     string
}

func (t *Token) String() string {
	if t.IsNil() {
		return ""
	}

	if t.s != "" {
		return t.s
	}

	var b []byte

	switch t.Code {
	case QWriteV1, QPartWrite:
		b = make([]byte, len(t.Bytes))
		copy(b, t.Bytes)
	case QWrite:
		// prefix + base58(uvarint(len(QID) | QID |
		//                 uvarint(len(NID) | NID |
		//                 uvarint(len(RAND_BYTES) | RAND_BYTES)
		b = make([]byte,
			byteutil.LenUvarInt(uint64(len(t.QID)))+
				len(t.QID)+
				byteutil.LenUvarInt(uint64(len(t.NID)))+
				len(t.NID)+
				byteutil.LenUvarInt(uint64(len(t.Bytes)))+
				len(t.Bytes))
		off := 0
		off += binary.PutUvarint(b[off:], uint64(len(t.QID)))
		off += copy(b[off:], t.QID)
		off += binary.PutUvarint(b[off:], uint64(len(t.NID)))
		off += copy(b[off:], t.NID)
		off += binary.PutUvarint(b[off:], uint64(len(t.Bytes)))
		off += copy(b[off:], t.Bytes)
	}

	t.s = t.prefix() + base58.Encode(b)

	return t.s
}

// AssertCode checks whether the hash's code equals the provided code
func (t *Token) AssertCode(c Code) error {
	if t == nil {
		return errors.E("token code check", errors.K.Invalid,
			"reason", "token is nil")
	}
	if t.Code != c {
		return errors.E("token code check", errors.K.Invalid,
			"expected", codeToPrefix[c],
			"actual", t.prefix())
	}
	return nil
}

func (t *Token) IsNil() bool {
	return t == nil
}

func (t *Token) IsValid() bool {
	return t != nil && t.Code != Unknown && len(t.Bytes) > 0
}

func (t *Token) prefix() string {
	p, found := codeToPrefix[t.Code]
	if !found {
		return codeToPrefix[Unknown]
	}
	return p
}

// MarshalText implements custom marshaling using the string representation.
func (t *Token) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

// UnmarshalText implements custom unmarshaling from the string representation.
func (t *Token) UnmarshalText(text []byte) error {
	parsed, err := FromString(string(text))
	if err != nil {
		return errors.E("unmarshal token", errors.K.Invalid, err)
	}
	*t = *parsed
	return nil
}

// Equal returns true if this token is equal to the given token.
func (t *Token) Equal(o *Token) bool {
	if t == nil {
		return o == nil
	}
	if o == nil {
		return false
	}
	if t.s != "" && o.s != "" {
		return t.s == o.s
	}
	return t.Code == o.Code &&
		bytes.Equal(t.Bytes, o.Bytes) &&
		t.QID.Equal(o.QID) &&
		t.NID.Equal(o.NID)
}

// Describe returns a textual description of this token.
func (t *Token) Describe() string {
	sb := strings.Builder{}
	sb.WriteString("type:   " + t.Code.Describe() + "\n")
	sb.WriteString("qid:    " + t.QID.String() + "\n")
	sb.WriteString("nid:    " + t.NID.String() + "\n")
	sb.WriteString("random: 0x" + hex.EncodeToString(t.Bytes) + "\n")
	return sb.String()
}

func (t *Token) Validate() (err error) {
	if !t.QID.IsNil() {
		err = t.QID.AssertCode(id.Q)
	}
	if err == nil && !t.NID.IsNil() {
		err = t.QID.AssertCode(id.Q)
	}
	if err == nil && len(t.Bytes) == 0 {
		err = errors.E("validate", errors.K.Invalid, "reason", "no random bytes")
	}
	return err
}

// Generate creates a random Token for the given Token type.
func Generate(code Code) *Token {
	return New(code, nil, nil)
}

// FromString parses a token from the given string representation. Alias for
// Parse().
func FromString(s string) (*Token, error) {
	return Parse(s)
}

// MustParse parses a token from the given string representation. Panics if the
// string cannot be parsed.
func MustParse(s string) *Token {
	res, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return res
}

// Parse parses a token from the given string representation.
func Parse(s string) (*Token, error) {
	e := errors.Template("parse token", errors.K.Invalid, "string", s)

	if len(s) < prefixLen {
		return nil, e("reason", "unknown prefix")
	}

	code, found := prefixToCode[s[:prefixLen]]
	if !found {
		return nil, e("reason", "unknown prefix")
	}

	if len(s) == prefixLen {
		return nil, e("reason", "too short")
	}

	dec, err := base58.Decode(s[prefixLen:])
	if err != nil {
		return nil, e(err)
	}

	switch code {
	case QWriteV1, QPartWrite:
		return &Token{Code: code, Bytes: dec, s: s}, nil
	case QWrite:
		// prefix + base58(uvarint(len(QID) | QID |
		//                 uvarint(len(NID) | NID |
		//                 uvarint(len(RAND_BYTES) | RAND_BYTES)
		res := &Token{Code: code, s: s}
		r := bytes.NewReader(dec)
		var n uint64

		decodeID := func() (res []byte, err error) {
			n, err = binary.ReadUvarint(r)
			if err != nil || n <= 0 || n > 50 {
				return nil, errors.E("decode id", errors.K.Invalid, err, "reason", "invalid size", "size", n)
			}
			res = make([]byte, n)
			read, err := r.Read(res)
			if err == io.EOF {
				return nil, e("decode id", "reason", "id truncated", "expected", n, "actual", read)
			}
			return res, nil
		}

		res.QID, err = decodeID()
		if err != nil {
			return nil, e(err, "reason", "invalid qid")
		}

		res.NID, err = decodeID()
		if err != nil {
			return nil, e(err, "reason", "invalid nid")
		}

		res.Bytes, err = decodeID()
		if err != nil {
			return nil, e(err, "reason", "invalid rand bytes")
		}

		err = res.Validate()
		if err != nil {
			return nil, e(err)
		}

		return res, nil
	}
	return nil, errors.E("FromString", errors.K.Invalid,
		"reason", "unknown code",
		"code", code)
}
