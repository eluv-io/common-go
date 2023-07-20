package drm

import (
	"bytes"
	"encoding/hex"

	"github.com/mr-tron/base58/base58"

	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"

	"github.com/eluv-io/common-go/format/hash"
)

const idSize = 16

// Code is the type of a DRM key
type Code uint8

func (c Code) String() string {
	return codeToPrefix[c]
}

// FromString parses the given string and returns the DRM key. Returns an error if the string is not a DRM key or a DRM
// key ofthe wrong type.
func (c Code) FromString(s string) (*KeyID, error) {
	k, err := FromString(s)
	if err != nil {
		return nil, err
	}
	return k, k.AssertCode(c)
}

// MustParse parses a DRM key from the given string representation. Panics if the string cannot be parsed.
func (c Code) MustParse(s string) *KeyID {
	res, err := c.FromString(s)
	if err != nil {
		panic(err)
	}
	return res
}

// lint disable
const (
	UNKNOWN Code = iota
	Key
)

const codeLen = 1
const prefixLen = 4

var codeToPrefix = map[Code]string{}
var prefixToCode = map[string]Code{
	"dukn": UNKNOWN,
	"drm_": Key,
}

func init() {
	for prefix, code := range prefixToCode {
		if len(prefix) != prefixLen {
			log.Fatal("invalid drm key prefix definition", "prefix", prefix)
		}
		codeToPrefix[code] = prefix
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// KeyID is the concatenation of a key ID and an associated content hash.
//     Key format : id (16 bytes) | digest (var bytes) | size (var bytes) | id (var bytes)
type KeyID struct {
	Code Code
	ID   []byte
	Hash *hash.Hash
	s    string
}

// New creates a new DRM key with the given code, ID, and hash.
// For code Key, id is expected to have a length of 16 bytes; h is expected to have a type of {Q, Unencrypted}
func New(c Code, id []byte, h *hash.Hash) (*KeyID, error) {
	e := errors.TemplateNoTrace("init drm key", errors.K.Invalid)
	if _, ok := codeToPrefix[c]; !ok {
		return nil, e("reason", "invalid code", "code", c)
	} else if c == UNKNOWN {
		return nil, nil
	} else if c != Key {
		return nil, e("reason", "code not supported", "code", c)
	} else if len(id) != idSize {
		return nil, e("reason", "invalid id", "id", id)
	} else if h == nil || h.IsNil() || h.Type.Code != hash.Q {
		return nil, e("reason", "invalid hash", "hash", h)
	}
	k := &KeyID{Code: c, ID: id, Hash: h}
	k.s = k.String()
	return k, nil
}

// FromString parses a DRM key from the given string representation.
func FromString(s string) (*KeyID, error) {
	e := errors.TemplateNoTrace("parse drm key", errors.K.Invalid, "string", s)
	if s == "" {
		return nil, nil
	} else if len(s) < prefixLen {
		return nil, e("reason", "invalid token string")
	}
	c, found := prefixToCode[s[:prefixLen]]
	if !found || c == UNKNOWN {
		return nil, e("reason", "invalid prefix")
	}
	b, err := base58.Decode(s[prefixLen:])
	if err != nil {
		return nil, e(err)
	}
	n := 0
	// Parse id
	m := idSize
	if n + m > len(b) {
		return nil, e("reason", "invalid id")
	}
	id := b[n : n+m]
	n += m
	// Parse hash
	h, err := hash.FromDecodedBytes(hash.Type{hash.Q, hash.Unencrypted}, b[n:])
	if err != nil {
		return nil, e(err, "reason", "invalid hash")
	}
	return &KeyID{Code: c, ID: id, Hash: h, s: s}, nil
}

// MustParse parses a DRM key from the given string representation. Panics if the string cannot be parsed.
func MustParse(s string) *KeyID {
	res, err := FromString(s)
	if err != nil {
		panic(err)
	}
	return res
}

func (k *KeyID) IsNil() bool {
	return k == nil || k.Code == UNKNOWN
}

func (k *KeyID) AssertCode(c Code) error {
	kcode := UNKNOWN
	if !k.IsNil() {
		kcode = k.Code
	}
	if kcode != c {
		return errors.NoTrace("verify drm key", errors.K.Invalid, "reason", "drm key code doesn't match", "expected", c, "actual", kcode)
	}
	return nil
}

func (k *KeyID) String() string {
	if k.IsNil() {
		return ""
	}
	if k.s == "" && len(k.ID) > 0 {
		b := make([]byte, len(k.ID))
		copy(b, k.ID)
		b = append(b, k.Hash.DecodedBytes()...)
		k.s = codeToPrefix[k.Code] + base58.Encode(b)
	}
	return k.s
}

// MarshalText converts this DRM key to text.
func (k KeyID) MarshalText() ([]byte, error) {
	return []byte(k.String()), nil
}

// UnmarshalText parses the DRM key from the given text.
func (k *KeyID) UnmarshalText(text []byte) error {
	parsed, err := FromString(string(text))
	if err != nil {
		return errors.NoTrace("unmarshal drm key", err)
	}
	if parsed == nil {
		// empty string parses to nil hash... best we can do is ignore it...
		return nil
	}
	*k = *parsed
	return nil
}

// Equal returns true if this DRM key is equal to the provided DRM key, false otherwise.
func (k *KeyID) Equal(k2 *KeyID) bool {
	if k == k2 {
		return true
	} else if k == nil || k2 == nil {
		return false
	}
	return k.String() == k2.String()
}

// AssertEqual returns nil if this DRM key is equal to the provided DRM key, an error with detailed reason otherwise.
func (k *KeyID) AssertEqual(k2 *KeyID) error {
	e := errors.TemplateNoTrace("assert drm keys equal", errors.K.Invalid, "expected", k, "actual", k2)
	switch {
	case k == k2:
		return nil
	case k == nil || k2 == nil:
		return e()
	case k.Code != k2.Code:
		return e("reason", "code differs")
	case !bytes.Equal(k.ID, k2.ID):
		return e("reason", "id differs", "expected_id", hex.EncodeToString(k.ID), "actual_id", hex.EncodeToString(k2.ID))
	case !k.Hash.Equal(k2.Hash):
		return e(k.Hash.AssertEqual(k2.Hash), "reason", "hash differs")
	default:
		return nil
	}
}
