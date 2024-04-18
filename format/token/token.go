package token

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mr-tron/base58/base58"

	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"

	"github.com/eluv-io/common-go/format/encryption"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/util/byteutil"
)

func NewObject(code Code, qid id.ID, nid id.ID, bytes ...byte) (*Token, error) {
	e := errors.Template("init token", errors.K.Invalid)
	if code == UNKNOWN {
		return nil, nil
	} else if code != QWriteV1 && code != QWrite {
		return nil, e("reason", "code not supported", "code", code)
	}
	res := &Token{Code: code}
	if len(bytes) == 0 {
		bytes = byteutil.RandomBytes(16)
	}
	res.Bytes = bytes
	if code == QWrite {
		if qid.AssertCode(id.Q) != nil {
			return nil, e("reason", "invalid qid", "qid", qid)
		}
		res.QID = qid
		if nid.AssertCode(id.QNode) != nil {
			return nil, e("reason", "invalid nid", "nid", nid)
		}
		res.NID = nid
	}
	res.MakeString()
	return res, nil
}

func NewPart(code Code, scheme encryption.Scheme, flags byte, bytes ...byte) (*Token, error) {
	e := errors.Template("init token", errors.K.Invalid)
	if code == UNKNOWN {
		return nil, nil
	} else if code != QPartWriteV1 && code != QPartWrite {
		return nil, e("reason", "code not supported", "code", code)
	}
	res := &Token{Code: code}
	if len(bytes) == 0 {
		bytes = byteutil.RandomBytes(16)
	}
	res.Bytes = bytes
	if code == QPartWrite {
		if !encryption.Schemes[scheme] {
			return nil, e("reason", "invalid scheme", "scheme", scheme)
		}
		res.Scheme = scheme
		if ValidateFlags(flags, allQPWFlags) {
			return nil, e("reason", "invalid flags", "flags", flags)
		}
		res.Flags = flags
	}
	res.MakeString()
	return res, nil
}

func NewLRO(code Code, nid id.ID, bytes ...byte) (*Token, error) {
	e := errors.Template("init token", errors.K.Invalid)
	if code == UNKNOWN {
		return nil, nil
	} else if code != LRO {
		return nil, e("reason", "code not supported", "code", code)
	}
	res := &Token{Code: code}
	if len(bytes) == 0 {
		bytes = byteutil.RandomBytes(16)
	}
	res.Bytes = bytes
	if nid.AssertCode(id.QNode) != nil {
		return nil, e("reason", "invalid nid", "nid", nid)
	}
	res.NID = nid
	res.MakeString()
	return res, nil
}

// Code is the type of a Token
type Code uint8

func CodeFromString(s string) (Code, error) {
	c, ok := prefixToCode[s]
	if !ok {
		return UNKNOWN, errors.E("parse code", errors.K.Invalid, "string", s)
	}
	return c, nil
}

func (c Code) String() string {
	return codeToPrefix[c]
}

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
	UNKNOWN      Code = iota
	QWriteV1          // 1st version: random bytes
	QWrite            // 2nd version: content ID, node ID, random bytes
	QPartWriteV1      // 1st version: random bytes
	QPartWrite        // 2nd version: scheme, flags, random bytes
	LRO               // node ID, random bytes
)

const prefixLen = 4

var codeToPrefix = map[Code]string{}
var prefixToCode = map[string]Code{
	"tunk": UNKNOWN,
	"tqw_": QWriteV1,
	"tq__": QWrite, // QWrite new version
	"tqpw": QPartWriteV1,
	"tqp_": QPartWrite, // QPartWrite new version
	"tlro": LRO,
}
var codeToName = map[Code]string{
	UNKNOWN:      "unknown",
	QWriteV1:     "content write token v1",
	QWrite:       "content write token",
	QPartWriteV1: "content part write token v1",
	QPartWrite:   "content part write token",
	LRO:          "bitcode LRO handle",
}

// NOTE: 5 char prefix - 2 underscores!
// This is a backward compatibility hack because the JS client code requires
// a "tqw_" prefix for tokens! This prefix will be switched back to the
// original "tq__" in the near future and
const qwPrefix = "tqw__"

const (
	NoQPWFlag       byte = 0b0
	PreambleQPWFlag      = 0b1             // Preamble exists
	allQPWFlags          = PreambleQPWFlag // Combination of all flags
)

var QPWFlagNames = map[byte]string{}
var QPWFlags = map[string]byte{
	"preamble": PreambleQPWFlag,
}

func DescribeFlags(flags byte, flagNames map[byte]string) string {
	sb := strings.Builder{}
	sb.WriteString("[")
	f := make([]string, 0, len(flagNames))
	for flag, name := range flagNames {
		if flag&flags > 0 {
			f = append(f, name)
		}
	}
	sort.Strings(f)
	sb.WriteString(strings.Join(f, ",") + "]")
	return sb.String()
}

func ValidateFlags(flags byte, allFlags byte) bool {
	return flags|allFlags > allFlags
}

func init() {
	for prefix, code := range prefixToCode {
		if len(prefix) != prefixLen {
			log.Fatal("invalid Token prefix definition", "prefix", prefix)
		}
		codeToPrefix[code] = prefix
	}
	codeToPrefix[QWrite] = qwPrefix // use "tqw__" when encoding!
	for name, code := range QPWFlags {
		QPWFlagNames[code] = name
	}
}

// Token is the type representing a Token. Tokens follow the multiformat
// principle and are prefixed with their type (a varint). Unlike other
// multiformat implementations like multihash, the type is serialized to textual
// form (String(), JSON) as a short text prefix instead of their encoded varint
// for increased readability.
type Token struct {
	Code   Code
	Bytes  []byte
	QID    id.ID             // content ID
	NID    id.ID             // node ID of insertion point node
	Scheme encryption.Scheme // encryption scheme
	Flags  byte              // misc flags
	s      string
}

func (t *Token) String() string {
	if t.IsNil() {
		return ""
	}

	if t.s != "" {
		return t.s
	}
	// we should never go there when t.s is computed in constructor
	return t.MakeString()
}

// MakeString recomputes the internal cached string representation of this Token
// MakeString is not safe for calls from concurrent go-routines.
func (t *Token) MakeString() string {
	var b []byte

	switch t.Code {
	case UNKNOWN:
		return ""
	case QWriteV1, QPartWriteV1:
		b = make([]byte, len(t.Bytes))
		copy(b, t.Bytes)
	case QWrite, LRO:
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
	case QPartWrite:
		// prefix + base58(byte(SCHEME) | byte(FLAGS) |
		//                 uvarint(len(RAND_BYTES) | RAND_BYTES)
		b = make([]byte, 2+byteutil.LenUvarInt(uint64(len(t.Bytes)))+len(t.Bytes))
		off := 0
		off += copy(b[off:], []byte{byte(t.Scheme)})
		off += copy(b[off:], []byte{t.Flags})
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
	return t != nil && t.Code != UNKNOWN && len(t.Bytes) > 0
}

func (t *Token) prefix() string {
	p, found := codeToPrefix[t.Code]
	if !found {
		return codeToPrefix[UNKNOWN]
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
		t.NID.Equal(o.NID) &&
		t.Scheme == o.Scheme &&
		t.Flags == o.Flags
}

// Describe returns a textual description of this token.
func (t *Token) Describe() string {
	sb := strings.Builder{}

	add := func(s string) {
		sb.WriteString(s)
		sb.WriteString("\n")
	}

	add("type:   " + t.Code.Describe())
	add("bytes:  0x" + hex.EncodeToString(t.Bytes))
	if t.Code == QWrite {
		add("qid:    " + t.QID.String())
	}
	if t.Code == QWrite || t.Code == LRO {
		add("nid:    " + t.NID.String())
	}
	if t.Code == QPartWrite {
		add("scheme: " + t.Scheme.String())
		add("flags:  " + DescribeFlags(t.Flags, QPWFlagNames))
	}
	return sb.String()
}

func (t *Token) Validate() (err error) {
	e := errors.Template("validate token", errors.K.Invalid)
	if t.Code == QWrite {
		err = t.QID.AssertCode(id.Q)
	}
	if err == nil && (t.Code == QWrite || t.Code == LRO) {
		err = t.NID.AssertCode(id.QNode)
	}
	if err == nil && t.Code == QPartWrite {
		if !encryption.Schemes[t.Scheme] {
			err = e("reason", "invalid scheme", "scheme", t.Scheme)
		}
		if err == nil && ValidateFlags(t.Flags, allQPWFlags) {
			err = e("reason", "invalid flags", "flags", t.Flags)
		}
	}
	if err == nil && len(t.Bytes) == 0 {
		err = e("reason", "no random bytes")
	}
	return err
}

// Generate creates a random Token for the given Token type.
func Generate(code Code) *Token {
	var t *Token
	switch code {
	case QWriteV1, QWrite:
		t, _ = NewObject(code, id.Generate(id.Q), id.Generate(id.QNode))
	case QPartWriteV1, QPartWrite:
		t, _ = NewPart(code, encryption.None, 0)
	case LRO:
		t, _ = NewLRO(code, id.Generate(id.QNode))
	}
	return t
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

	var code Code
	var found bool
	prefix := s[:prefixLen]

	if strings.HasPrefix(s, qwPrefix) {
		prefix = qwPrefix
		code = QWrite
		found = true
	} else {
		code, found = prefixToCode[prefix]
		if !found {
			return nil, e("reason", "unknown prefix")
		}
	}
	if len(s) == len(prefix) {
		return nil, e("reason", "too short")
	}

	dec, err := base58.Decode(s[len(prefix):])
	if err != nil {
		return nil, e(err)
	}

	switch code {
	case QWriteV1, QPartWriteV1:
		return &Token{Code: code, Bytes: dec, s: s}, nil
	case QWrite, LRO:
		// prefix + base58(uvarint(len(QID) | QID |
		//                 uvarint(len(NID) | NID |
		//                 uvarint(len(RAND_BYTES) | RAND_BYTES)
		res := &Token{Code: code, s: s}
		r := bytes.NewReader(dec)

		res.QID, err = decodeBytes(r)
		if err != nil {
			return nil, e(err, "reason", "invalid qid")
		}

		res.NID, err = decodeBytes(r)
		if err != nil {
			return nil, e(err, "reason", "invalid nid")
		}

		res.Bytes, err = decodeBytes(r)
		if err != nil {
			return nil, e(err, "reason", "invalid rand bytes")
		}

		err = res.Validate()
		if err != nil {
			return nil, e(err)
		}

		return res, nil
	case QPartWrite:
		// prefix + base58(byte(SCHEME) | byte(FLAGS) |
		//                 uvarint(len(RAND_BYTES) | RAND_BYTES)
		res := &Token{Code: code, s: s}

		if len(dec) < 3 {
			return nil, e(err, "reason", "token truncated")
		}
		res.Scheme = encryption.Scheme(dec[0])
		res.Flags = dec[1]

		res.Bytes, err = decodeBytes(bytes.NewReader(dec[2:]))
		if err != nil {
			return nil, e(err, "reason", "invalid rand bytes")
		}

		err = res.Validate()
		if err != nil {
			return nil, e(err)
		}

		return res, nil
	}
	return nil, e("reason", "unknown code", "code", code)
}

func decodeBytes(r *bytes.Reader) ([]byte, error) {
	e := errors.Template("decode bytes", errors.K.Invalid)
	n, err := binary.ReadUvarint(r)
	if err != nil || n > 50 {
		return nil, e(err, "reason", "invalid size", "size", n)
	}
	res := make([]byte, n)
	read, err := r.Read(res)
	if err == io.EOF {
		return nil, e("reason", "token truncated", "expected", n, "actual", read)
	}
	return res, nil
}
