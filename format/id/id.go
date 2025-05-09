package id

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/mr-tron/base58/base58"
	uuid "github.com/satori/go.uuid"

	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
)

// Code is the type of an ID
type Code uint8

// FromString parses the given string and returns the ID. Returns an error
// if the string is not a ID or an ID of the wrong type.
func (c Code) FromString(s string) (ID, error) {
	id, err := FromString(s)
	if err != nil {
		n, ok := codeToName[c]
		if !ok {
			n = fmt.Sprintf("Unknown code %d", c)
		}
		return nil, errors.E("parse ID", err, "expected_type", n)
	}
	return id, id.AssertCode(c)
}

// MustParse parses an ID from the given string representation. Panics if the
// string cannot be parsed.
func (c Code) MustParse(s string) ID {
	res, err := c.FromString(s)
	if err != nil {
		panic(err)
	}
	return res
}

// lint disable
const (
	UNKNOWN Code = iota
	Account
	User
	QLib
	Q
	QStateStore
	QSpace
	QFileUpload
	QFilesJob
	QNode
	Network
	KMS
	CachedResultSet
	Tenant
	Group
	Key
	Ed25519
)

const codeLen = 1
const prefixLen = 4

var codeToPrefix = map[Code]string{}
var prefixToCode = map[string]Code{
	"iukn": UNKNOWN,
	"iacc": Account, // @deprecated
	"iusr": User,    // @deprecated
	"ilib": QLib,
	"iq__": Q,
	"iqss": QStateStore,
	"ispc": QSpace,
	"iqfu": QFileUpload,
	"iqfj": QFilesJob,
	"inod": QNode,
	"inet": Network,
	"ikms": KMS,
	"icrs": CachedResultSet,
	"iten": Tenant,
	"igrp": Group,
	"ikey": Key,     // last 20 bytes of the keccak256 of a bls381 ecp
	"ied2": Ed25519, // 32 byte ed25519 public key
}
var codeToName = map[Code]string{
	UNKNOWN:         "unknown",
	Account:         "account",
	User:            "user",
	QLib:            "content library",
	Q:               "content",
	QStateStore:     "content state store",
	QSpace:          "content space",
	QFileUpload:     "content file upload",
	QFilesJob:       "content files job",
	QNode:           "fabric node",
	Network:         "network",
	KMS:             "KMS",
	CachedResultSet: "cached result set",
	Tenant:          "tenant",
	Group:           "group",
	Key:             "key",
	Ed25519:         "ed25519 public key",
}

func init() {
	for prefix, code := range prefixToCode {
		if len(prefix) != prefixLen {
			log.Fatal("invalid ID prefix definition", "prefix", prefix)
		}
		codeToPrefix[code] = prefix
	}
}

// ID is the type representing an ID. IDs follow the multiformat principle and
// are prefixed with their type (a varint). Unlike other multiformat
// implementations like multihash, the type is serialized to textual form
// (String(), JSON) as a short text prefix instead of their encoded varint for
// increased readability.
type ID []byte

func (id ID) String() string {
	if len(id) <= codeLen {
		return ""
	}
	return id.prefix() + base58.Encode(id[codeLen:])
}

// AssertCode checks whether the ID's code equals the provided code
func (id ID) AssertCode(c Code) error {
	if id == nil || id.Code() != c {
		return errors.NoTrace("ID code check", errors.K.Invalid,
			"expected", codeToPrefix[c],
			"actual", id.prefix())
	}
	return nil
}

func (id ID) prefix() string {
	p, found := codeToPrefix[id.Code()]
	if !found {
		return codeToPrefix[UNKNOWN]
	}
	return p
}

func (id ID) Code() Code {
	if id.IsNil() {
		return UNKNOWN
	}
	return Code(id[0])
}

// MarshalText implements custom marshaling using the string representation.
func (id ID) MarshalText() ([]byte, error) {
	return []byte(id.String()), nil
}

func (id ID) Bytes() []byte {
	if id.IsNil() {
		return nil
	}
	return id[codeLen:]
}

// UnmarshalText implements custom unmarshaling from the string representation.
func (id *ID) UnmarshalText(text []byte) error {
	parsed, err := FromString(string(text))
	if err != nil {
		return errors.NoTrace("unmarshal ID", errors.K.Invalid, err)
	}
	*id = parsed
	return nil
}

// As returns a copy of this ID with the given code as the type of the new ID.
func (id ID) As(c Code) ID {
	if !id.IsValid() {
		return nil
	}
	buf := make([]byte, len(id))
	copy(buf, id)
	buf[0] = byte(c)
	return buf
}

func (id ID) IsNil() bool {
	return len(id) == 0
}

func (id ID) IsValid() bool {
	return len(id) > codeLen
}

func (id ID) Is(s string) bool {
	sID, err := FromString(s)
	if err != nil {
		return false
	}
	return bytes.Equal(id, sID)
}

func (id ID) Equal(other ID) bool {
	return bytes.Equal(id, other)
}

// Equivalent returns true if this ID is equal to the given ID, ignoring the ID
// code.
func (id ID) Equivalent(other ID) bool {
	return bytes.Equal(id.Bytes(), other.Bytes())
}

// Generate creates a random ID for the given ID type.
func Generate(code Code) ID {
	return ID(append([]byte{byte(code)}, uuid.NewV4().Bytes()...))
}

func NewID(code Code, codeBytes []byte) ID {
	return ID(append([]byte{byte(code)}, codeBytes...))
}

func IsIDString(s string) bool {
	if len(s) <= prefixLen {
		return false
	}

	_, found := prefixToCode[s[:prefixLen]]
	if !found {
		return false
	}

	if len(s[prefixLen:]) == 0 {
		return false
	}

	return true
}

// FromString parses an ID from the given string representation. Alias for
// Parse().
func FromString(s string) (ID, error) {
	return Parse(s)
}

// MustParse parses an ID from the given string representation. Panics if the
// string cannot be parsed.
func MustParse(s string) ID {
	res, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return res
}

// Parse parses an ID from the given string representation.
func Parse(s string) (ID, error) {
	e := errors.TemplateNoTrace("parse ID", errors.K.Invalid, "string", s)
	if len(s) <= prefixLen {
		if len(s) == 0 {
			return nil, e("reason", "empty string")
		}
		return nil, e("reason", "invalid prefix")
	}

	code, found := prefixToCode[s[:prefixLen]]
	if !found {
		return nil, e("reason", "unknown prefix")
	}

	dec, err := base58.Decode(s[prefixLen:])
	if err != nil {
		return nil, e(err)
	}
	b := []byte{byte(code)}
	return ID(append(b, dec...)), nil
}

func FormatId(id string, idType Code) (string, error) {
	qid, err := FromString(id)
	if err == nil { // Assume content fabric format
		return "0x" + hex.EncodeToString(qid.Bytes()), nil
	} else { // Assume hex format
		hexPrefix := "0x"
		if strings.HasPrefix(id, hexPrefix) {
			id = id[len(hexPrefix):]
		}
		data, err := hex.DecodeString(id)
		if err != nil {
			return "", err
		}
		qid = append([]byte{0}, data...)
		return qid.As(idType).String(), nil
	}
}

func FromStringValidate(s string, valCode Code) (ID, error) {
	id, err := FromString(s)
	if err != nil {
		return nil, err
	}
	if id.Code() != valCode {
		return nil, errors.NoTrace("invalid code", errors.K.Invalid,
			"expect", valCode,
			"actual", id.Code())
	}
	return id, nil
}

func CodeFromPrefix(maybePrefix string) Code {
	maybeCode, ok := prefixToCode[strings.ToLower(maybePrefix)]
	if !ok {
		return UNKNOWN
	}
	return maybeCode
}
