package hash

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"

	"github.com/qluvio/content-fabric/errors"
	ei "github.com/qluvio/content-fabric/format/id"
	"github.com/qluvio/content-fabric/log"

	"github.com/mr-tron/base58/base58"
)

// Code is the code of a hash
type Code uint8

// lint disable
const (
	UNKNOWN Code = iota
	Q
	QPart
	QPartLive
)

// FromString parses the given string and returns the hash. Returns an error
// if the string is not a hash or a hash of the wrong code.
func (c Code) FromString(s string) (*Hash, error) {
	h, err := FromString(s)
	if err != nil {
		n, ok := codeToName[c]
		if !ok {
			n = fmt.Sprintf("Unknown code %d", c)
		}
		return nil, errors.E("parse Hash", err, "expected_type", n)
	}
	return h, h.AssertCode(c)
}

///////////////////////////////////////////////////////////////////////////////

// Format is the format of a hash
type Format uint8

const (
	Unencrypted Format = iota // SHA256, No encryption
	AES128AFGH                // SHA256, AES-128, AFGHG BLS12-381, 1 MB block size
)

// FromString parses the given string and returns the hash. Returns an error
// if the string is not a hash or a hash of the wrong code.
func (f Format) FromString(s string) (*Hash, error) {
	h, err := FromString(s)
	if err != nil {
		return nil, err
	}
	return h, h.AssertFormat(f)
}

///////////////////////////////////////////////////////////////////////////////

// Type, the composition of Code and Format, is the type of a hash
type Type struct {
	Code   Code
	Format Format
}

func TypeFromString(s string) (Type, error) {
	t, ok := prefixToType[s]
	if !ok {
		return Type{}, errors.E("parse type", errors.K.Invalid, "string", s)
	}
	return t, nil
}

func (t Type) String() string {
	return typeToPrefix[t]
}

const prefixLen = 4

var typeToPrefix = map[Type]string{}
var prefixToType = map[string]Type{
	"hunk": Type{UNKNOWN, Unencrypted},
	"hq__": Type{Q, Unencrypted},
	"hqp_": Type{QPart, Unencrypted},
	"hqpe": Type{QPart, AES128AFGH},
	"hql_": Type{QPartLive, Unencrypted},
	"hqle": Type{QPartLive, AES128AFGH},
}
var codeToName = map[Code]string{
	UNKNOWN: "unknown",
	Q:       "content",
	QPart:   "content part",
}

func init() {
	for p, t := range prefixToType {
		if len(p) != prefixLen {
			log.Fatal("invalid hash prefix definition", "prefix", p)
		}
		typeToPrefix[t] = p
	}
}

///////////////////////////////////////////////////////////////////////////////

// Hash is the output of a cryptographic hash function and associated metadata, identifying a particular
// instance of an immutable resource.
// Q format : type (1 byte) | digest (var bytes) | size (var bytes) | id (var bytes)
// QPart format : type (1 byte) | digest (var bytes) | size (var bytes)
// QPartLive format : type (1 byte) | digest (var bytes)
type Hash struct {
	Type   Type
	Digest []byte
	Size   int64
	ID     ei.ID
	s      string
}

// New creates a new hash with the given type, digest, size, and ID
func New(htype Type, digest []byte, size int64, id ei.ID) (*Hash, error) {
	if _, ok := typeToPrefix[htype]; !ok {
		return nil, errors.E("init hash", errors.K.Invalid, "reason", "invalid type", "code", htype.Code, "format", htype.Format)
	} else if htype.Code == UNKNOWN {
		return nil, nil
	}
	e := errors.Template("init hash", errors.K.Invalid)

	if htype.Code != QPartLive && len(digest) != sha256.Size {
		return nil, errors.E("init hash", errors.K.Invalid, "reason", "invalid digest", "digest", digest)
	}

	if size < 0 {
		return nil, e("reason", "invalid size", "size", size)
	}

	switch htype.Code {
	case Q:
		if id.AssertCode(ei.Q) != nil {
			return nil, e("reason", "invalid id", "id", id)
		}
	case QPart:
		if len(id) > 0 {
			return nil, e("reason", "id not supported with QPart")
		}
	}

	h := &Hash{Type: htype, Digest: digest, Size: size, ID: id}
	h.s = h.String()

	return h, nil
}

// FromString parses a hash from the given string representation.
func FromString(s string) (*Hash, error) {
	if s == "" {
		return nil, nil
	}
	e := errors.Template("parse hash", errors.K.Invalid, "string", s)

	if len(s) <= prefixLen {
		return nil, e("reason", "invalid string")
	}

	htype, found := prefixToType[s[:prefixLen]]
	if !found {
		return nil, e("reason", "invalid prefix")
	}

	b, err := base58.Decode(s[prefixLen:])
	if err != nil {
		return nil, e(err, "string", s)
	}
	n := 0

	var digest []byte
	var size int64
	var id ei.ID
	if htype.Code != QPartLive {
		// Parse digest
		m := sha256.Size
		if n+m > len(b) {
			return nil, errors.E("parse hash", errors.K.Invalid, "reason", "invalid digest", "string", s)
		}
		digest = b[n : n+m]
		n += m

		// Parse size
		sz, m := binary.Uvarint(b[n:])
		if m <= 0 {
			return nil, errors.E("parse hash", errors.K.Invalid, "reason", "invalid size", "string", s)
		}
		size = int64(sz)
		n += m

		if htype.Code == Q {
			// Parse id
			id = ei.NewID(ei.Q, b[n:])
		}
	} else {
		// Parse digest
		digest = b[n:]
	}

	return &Hash{Type: htype, Digest: digest, Size: size, ID: id, s: s}, nil
}

func (h *Hash) String() string {
	if h.IsNil() {
		return ""
	}

	if h.s == "" && len(h.Digest) > 0 {
		b := make([]byte, len(h.Digest))
		copy(b, h.Digest)

		if h.Type.Code != QPartLive {
			s := make([]byte, binary.MaxVarintLen64)
			n := binary.PutUvarint(s, uint64(h.Size))
			b = append(b, s[:n]...)

			if h.Type.Code == Q && h.ID.IsValid() {
				b = append(b, h.ID.Bytes()...)
			}
		}

		h.s = h.prefix() + base58.Encode(b)
	}

	return h.s
}

func (h *Hash) IsNil() bool {
	return h == nil || h.Type.Code == UNKNOWN
}

// AssertType checks whether the hash's type equals the provided type
func (h *Hash) AssertType(t Type) error {
	if h.IsNil() || h.Type != t {
		return errors.E("verify hash", errors.K.Invalid, "reason", "hash type doesn't match",
			"expected", typeToPrefix[t],
			"actual", h.prefix())
	}
	return nil
}

func (h *Hash) AssertCode(c Code) error {
	hcode := UNKNOWN
	if !h.IsNil() {
		hcode = h.Type.Code
	}
	if hcode != c {
		return errors.E("verify hash", errors.K.Invalid, "reason", "hash code doesn't match",
			"expected", c,
			"actual", hcode)
	}
	return nil
}

func (h *Hash) AssertFormat(f Format) error {
	hformat := Unencrypted
	if !h.IsNil() {
		hformat = h.Type.Format
	}
	if hformat != f {
		return errors.E("verify hash", errors.K.Invalid, "reason", "hash format doesn't match",
			"expected", f,
			"actual", hformat)
	}
	return nil
}

func (h *Hash) prefix() string {
	var p string
	var found bool
	if !h.IsNil() {
		p, found = typeToPrefix[h.Type]
	}
	if !found {
		return typeToPrefix[Type{UNKNOWN, Unencrypted}]
	}
	return p
}

// MarshalText converts this hash to text.
func (h Hash) MarshalText() ([]byte, error) {
	return []byte(h.String()), nil
}

// UnmarshalText parses the hash from the given text.
func (h *Hash) UnmarshalText(text []byte) error {
	parsed, err := FromString(string(text))
	if err != nil {
		return errors.E("unmarshal hash", err)
	}
	if parsed == nil {
		// empty string parses to nil hash... best we can do is ignore it...
		return nil
	}
	*h = *parsed
	return nil
}

// As returns a copy of this hash with the given code as the type of the new
// hash.
func (h *Hash) As(c Code, id ei.ID) (*Hash, error) {
	if h.IsNil() || c == h.Type.Code {
		return h, nil
	} else if _, ok := typeToPrefix[Type{c, h.Type.Format}]; !ok {
		return nil, errors.E("convert hash", errors.K.Invalid, "reason", "invaid type", "code", c, "format", h.Type.Format)
	}
	var res Hash = *h
	res.Type.Code = c
	res.s = ""
	if c != Q {
		res.ID = nil
	} else if id != nil && id.AssertCode(ei.Q) == nil {
		res.ID = id
	} else {
		return nil, errors.E("convert hash", errors.K.Invalid, "reason", "invalid id", "id", id)
	}
	res.s = res.String()
	return &res, nil
}

// Equal returns true if this hash is equal to the provided hash, false
// otherwise.
func (h *Hash) Equal(h2 *Hash) bool {
	if h == h2 {
		return true
	} else if h == nil || h2 == nil {
		return false
	}
	return h.String() == h2.String()
}

// AssertEqual returns nil if this hash is equal to the provided hash, an error
// with detailed reason otherwise.
func (h *Hash) AssertEqual(h2 *Hash) error {
	e := errors.Template("hash.assert-equal", errors.K.Invalid, "expected", h, "actual", h2)
	if h == h2 {
		return nil
	} else if h == nil || h2 == nil {
		return e()
	}
	if h.Type != h2.Type {
		return e("reason", "type differs")
	}
	if h.Size != h2.Size {
		return e("reason", "size differs", "expected_size", h.Size, "actual_size", h2.Size)
	}
	if h.ID.String() != h2.ID.String() {
		return e("reason", "ID differs", "expected_id", h.ID, "actual_id", h2.ID)
	}
	if !bytes.Equal(h.Digest, h2.Digest) {
		return e("reason", "digest differs",
			"expected_digest", hex.EncodeToString(h.Digest),
			"actual_digest", hex.EncodeToString(h2.Digest))
	}
	return nil
}

func (h *Hash) DigestBytes() []byte {
	if h.IsNil() {
		return nil
	}
	return h.Digest
}

///////////////////////////////////////////////////////////////////////////////

// Digest encapsulates a message digest function which produces a specific type
// of Hash
type Digest struct {
	hash.Hash
	htype Type
	id    ei.ID
	size  int64
}

// make sure Digest implements the Hash interface
var _ hash.Hash = (*Digest)(nil)

// NewDigest creates a new digest
func NewDigest(h hash.Hash, t Type, i ei.ID) *Digest {
	return &Digest{Hash: h, htype: t, id: i}
}

func (d *Digest) Write(p []byte) (int, error) {
	n, err := d.Hash.Write(p)
	d.size += int64(n)
	return n, err
}

// AsHash finalizes the digest calculation using all the bytes that were
// previously written to this digest object and return the result as a Hash.
func (d *Digest) AsHash() *Hash {
	b := d.Hash.Sum(nil)
	h, err := New(d.htype, b, d.size, d.id)
	if err != nil {
		// errors must be caught by unit tests!
		log.Fatal("invalid hash", "error", err)
	}
	return h
}

///////////////////////////////////////////////////////////////////////////////

func CalcHash(reader io.Reader) (*Hash, error) {
	digest := NewDigest(sha256.New(), Type{QPart, Unencrypted}, nil)
	buf := make([]byte, 128*1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			_, err = digest.Write(buf[:n])
			if err != nil {
				return nil, err
			}
		}
		if err == io.EOF {
			err = nil
			break
		} else if err != nil {
			return nil, err
		}
	}
	return digest.AsHash(), nil
}
