package hash

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/mr-tron/base58/base58"

	ei "github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/util/byteutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

// Code is the code of a hash
type Code uint8

// lint disable
const (
	UNKNOWN            Code = iota
	Q                       // content object hash
	QPart                   // regular part hash
	QPartLive               // live part that generates a regular part upon finalization for vod
	QPartLiveTransient      // live part that doesn't generate a regular part upon finalization
	C                       // content object hash     --> including storage ID
	CPart                   // content part hash       --> including storage ID
	CPartLive               // live content part hash  --> including storage ID

	// live content part hash including storage ID that doesn't generate a regular part upon finalization
	// PENDING(LUK): is this really needed?
	CPartLiveTransient
)

func (c Code) IsContent() bool {
	return c == Q || c == C
}

func (c Code) IsPart() bool {
	return c == QPart ||
		c == CPart ||
		c == QPartLive ||
		c == QPartLiveTransient ||
		c == CPartLive ||
		c == CPartLiveTransient
}

func (c Code) IsLive() bool {
	return c == QPartLive ||
		c == QPartLiveTransient ||
		c == CPartLive ||
		c == CPartLiveTransient
}

func (c Code) hasStorageId() bool {
	return c == C || c == CPart || c == CPartLive || c == CPartLiveTransient
}

// FromString parses the given string and returns the hash.
// Returns an error if the string is not a hash or a hash of the wrong code.
func (c Code) FromString(s string) (*Hash, error) {
	h, err := FromString(s)
	if err != nil {
		return nil, err
	}
	return h, h.AssertCode(c)
}

// MustParse parses the given string and returns the hash.
// It panics on error.
func (c Code) MustParse(s string) *Hash {
	ret, err := c.FromString(s)
	if err != nil {
		panic(err)
	}
	return ret
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Format is the format of a hash: Unencrypted|AES128AFGH
type Format uint8

const (
	Unencrypted Format = iota // SHA256, No encryption
	AES128AFGH                // SHA256, AES-128, AFGHG BLS12-381, 1 MB block size
)

// FromString parses the given string and returns the hash.
// Returns an error if the string is not a hash or a hash of the wrong code.
func (f Format) FromString(s string) (*Hash, error) {
	h, err := FromString(s)
	if err != nil {
		return nil, err
	}
	return h, h.AssertFormat(f)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Type is the composition of Code and Format.
type Type struct {
	Code   Code
	Format Format
}

func TypeFromString(s string) (Type, error) {
	t, ok := prefixToType[s]
	if !ok {
		return Type{}, errors.NoTrace("parse type", errors.K.Invalid, "string", s)
	}
	return t, nil
}

func (t Type) String() string {
	return typeToPrefix[t]
}

func (t Type) Describe() string {
	var c, f string
	switch t.Code {
	case UNKNOWN:
		c = "unknown"
	case Q:
		c = "content"
	case QPart:
		c = "content part"
	case QPartLive:
		c = "live content part"
	case QPartLiveTransient:
		c = "transient live content part"
	case C:
		c = "content with storage id"
	case CPart:
		c = "content part with storage id"
	case CPartLive:
		c = "live content part with storage id"
	case CPartLiveTransient:
		c = "transient live content part"
	}
	switch t.Format {
	case Unencrypted:
		f = "unencrypted"
	case AES128AFGH:
		f = "encrypted with AES-128, AFGHG BLS12-381, 1 MB block size"
	}
	return c + ", " + f
}

func (t Type) IsValid() bool {
	if _, ok := typeToPrefix[t]; !ok {
		return false
	} else if t.Code == UNKNOWN {
		return false
	}
	return true
}

const prefixLen = 4

var typeToPrefix = map[Type]string{}
var prefixToType = map[string]Type{
	"hunk": {UNKNOWN, Unencrypted},
	"hq__": {Q, Unencrypted},
	"hqp_": {QPart, Unencrypted},
	"hqpe": {QPart, AES128AFGH},
	"hql_": {QPartLive, Unencrypted},
	"hqle": {QPartLive, AES128AFGH},
	"hqt_": {QPartLiveTransient, Unencrypted},
	"hqte": {QPartLiveTransient, AES128AFGH},
	"hc__": {C, Unencrypted},
	"hcp_": {CPart, Unencrypted},
	"hcpe": {CPart, AES128AFGH},
	"hcl_": {CPartLive, Unencrypted},
	"hcle": {CPartLive, AES128AFGH},
	"hct_": {CPartLiveTransient, Unencrypted},
	"hcte": {CPartLiveTransient, AES128AFGH},
}

func init() {
	for p, t := range prefixToType {
		if len(p) != prefixLen {
			log.Fatal("invalid hash prefix definition", "prefix", p)
		}
		typeToPrefix[t] = p
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Hash is the output of a cryptographic hash function and associated metadata, identifying a particular instance of an
// immutable resource.
//
// Formats:
//
//	Q:                  type (1 byte) | digest (var bytes) | size (var bytes) | id (var bytes)
//	QPart:              type (1 byte) | digest (var bytes) | size (var bytes) | preamble_size (var bytes, optional)
//	QPartLive:          type (1 byte) | expiration (var bytes) | digest (var bytes)
//	QPartLiveTransient: type (1 byte) | expiration (var bytes) | digest (var bytes)
//	(Old) QPartLive:    type (1 byte) | digest (24-25 bytes)
//	C:                  type (1 byte) | storage ID (var bytes) | digest (var bytes) | size (var bytes) | id (var bytes)
//	QPart:              type (1 byte) | storage ID (var bytes) | digest (var bytes) | size (var bytes) | preamble_size (var bytes, optional)
//	CPartLive:          type (1 byte) | storage ID (var bytes) | expiration (var bytes) | digest (var bytes)
//	CPartLiveTransient: type (1 byte) | storage ID (var bytes) | expiration (var bytes) | digest (var bytes)
//
// The preferred way to create regular part and content hashes is using the builder:
//
//	digest := hash.NewBuilder().WithStorageId(storageId).WithXyz(...)
//	digest.Write(data)
//	...
//	partHash, err := digest.BuildHash()
//
// This creates a regular content part hash. If this is the hash of the content object's qstruct part (or qref part for
// V1 objects), then the hash can be converted to a content hash using:
//
//	contentHash, err := partHash.AsContentHash(contentID)
//
// To derive the part hash from a content hash, use:
//
//	partHash, err := contentHash.AsPartHash()
//
// Live hashes are created using the NewLive function:
//
//	hash, err := NewLive(Type{QPartLive, Unencrypted}, digest, expiration)
type Hash struct {
	Type         Type
	Digest       []byte
	Size         int64
	PreambleSize int64
	ID           ei.ID
	Expiration   utc.UTC
	StorageId    uint
	s            string
}

// NewObject creates a new object hash with the given type, digest, size, and ID
//
// Deprecated: use NewBuilder().BuildHash(), then AsContentHash() instead.
func NewObject(htype Type, digest []byte, size int64, id ei.ID) (*Hash, error) {
	e := errors.TemplateNoTrace("init hash", errors.K.Invalid)
	if !htype.Code.IsContent() {
		return nil, e("reason", "code not supported", "code", htype.Code)
	}

	h := &Hash{Type: htype, Digest: digest, Size: size, ID: id}
	if err := h.Validate(); err != nil {
		return nil, e(err)
	}

	h.s = h.String()
	return h, nil
}

// NewPart creates a new non-live part hash with the given type, digest, size, and optional preamble size
//
// Deprecated: use NewBuilder().BuildHash() instead.
func NewPart(htype Type, digest []byte, size int64, preambleSize int64) (*Hash, error) {
	e := errors.TemplateNoTrace("NewPartHash", errors.K.Invalid)

	// this code is left unchanged for backwards compatibility
	if _, ok := typeToPrefix[htype]; !ok {
		return nil, e("reason", "invalid type", "code", htype.Code, "format", htype.Format)
	} else if htype.Code == UNKNOWN {
		return nil, nil
	} else if htype.Code != QPart {
		return nil, e("reason", "code not supported", "code", htype.Code)
	}

	if size < 0 {
		return nil, e("reason", "invalid part size", "size", size)
	}
	if preambleSize < 0 {
		return nil, e("reason", "invalid preamble size", "preamble_size", preambleSize)
	}

	return newPart(htype.Format, digest, uint64(size), uint64(preambleSize), 0)
}

// newPart creates a new non-live part hash with the given type, digest, size, preamble size and storage ID
func newPart(format Format, digest []byte, size uint64, preambleSize uint64, storageId uint) (*Hash, error) {
	e := errors.TemplateNoTrace("NewPartHash", errors.K.Invalid)

	if len(digest) != sha256.Size {
		return nil, e("reason", "invalid digest", "digest", digest)
	}

	code := QPart
	if storageId > 0 {
		code = CPart
	}

	h := &Hash{Type: Type{
		Code:   code,
		Format: format,
	}, Digest: digest, Size: int64(size), PreambleSize: int64(preambleSize), StorageId: storageId}

	if err := h.Validate(); err != nil {
		return nil, e(err)
	}

	h.s = h.String()
	return h, nil
}

// NewLive creates a new live hash with the given type, digest, and expiration
func NewLive(htype Type, digest []byte, expiration utc.UTC) (*Hash, error) {
	e := errors.TemplateNoTrace("init hash", errors.K.Invalid)
	if _, ok := typeToPrefix[htype]; !ok {
		return nil, e("reason", "invalid type", "code", htype.Code, "format", htype.Format)
	} else if htype.Code == UNKNOWN {
		return nil, nil
	} else if !htype.Code.IsLive() {
		return nil, e("reason", "code not supported", "code", htype.Code)
	} else if expiration.IsZero() { // Do not allow deprecated/legacy QPartLive format
		return nil, e("reason", "invalid expiration", "expiration", expiration)
	}

	// Strip sub-second info from expiration, since it is stored as Unix time in hash string
	expiration = expiration.Truncate(time.Second)

	h := &Hash{Type: htype, Digest: digest, Expiration: expiration}
	if err := h.Validate(); err != nil {
		return nil, e(err)
	}

	h.s = h.String()
	return h, nil
}

// MustParse parses a hash from the given string representation.
// Panics if the string cannot be parsed.
func MustParse(s string) *Hash {
	res, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return res
}

// Parse parses a hash from the given string representation.
func Parse(s string) (*Hash, error) {
	return FromString(s)
}

// FromString parses a hash from the given string representation.
func FromString(s string) (*Hash, error) {
	e := errors.TemplateNoTrace("parse hash", errors.K.Invalid, "string", s)

	if s == "" {
		return nil, nil
	}

	if len(s) <= prefixLen {
		return nil, e("reason", "invalid token string")
	}

	htype, found := prefixToType[s[:prefixLen]]
	if !found || htype.Code == UNKNOWN {
		return nil, e("reason", "invalid prefix")
	}

	b, err := base58.Decode(s[prefixLen:])
	if err != nil {
		return nil, e(err)
	}

	h, err := FromDecodedBytes(htype, b)
	if err != nil {
		return nil, e(err)
	}
	h.s = s

	return h, nil
}

// FromDecodedBytes parses a hash from the given base58-decoded bytes representation.
func FromDecodedBytes(htype Type, b []byte) (*Hash, error) {
	e := errors.TemplateNoTrace("parse hash", errors.K.Invalid)

	n := 0
	var digest []byte
	var size, preambleSize int64
	var id ei.ID
	var expiration utc.UTC
	var storageId uint64

	if htype.Code.hasStorageId() {
		// Parse storage ID
		var m int
		storageId, m = binary.Uvarint(b[n:])
		if m <= 0 {
			return nil, e("reason", "invalid storage ID")
		}
		n += m
	}

	if !htype.Code.IsLive() {
		// Parse digest
		m := sha256.Size
		if n+m > len(b) {
			return nil, e("reason", "invalid digest")
		}
		digest = b[n : n+m]
		n += m

		// Parse size
		var sz uint64
		sz, m = binary.Uvarint(b[n:])
		if m <= 0 {
			return nil, e("reason", "invalid size")
		}
		size = int64(sz)
		n += m

		if htype.Code == QPart {
			sz, m = binary.Uvarint(b[n:])
			if m < 0 {
				return nil, e("reason", "invalid preamble size")
			} else if m > 0 {
				preambleSize = int64(sz)
			}
		} else if htype.Code == Q {
			// Parse id
			id = ei.NewID(ei.Q, b[n:])
		}
	} else if len(b[n:]) == 24 || len(b[n:]) == 25 {
		// Legacy live part format, which did not include an expiration time. The size of the digest was 25 at first
		// with an incorrect 0 byte prefix that was later stripped and resulting in 24 bytes.
		// See https://github.com/qluvio/content-fabric/commit/9c961f978e217bee97cb33e8d4288d43ea8c8ba6
		digest = b[n:]
	} else {
		// Parse expiration
		exp, m := binary.Uvarint(b[n:])
		if m <= 0 {
			return nil, e("reason", "invalid expiration")
		}
		expiration = utc.Unix(int64(exp), 0)
		n += m

		// Parse digest
		digest = b[n:]
	}

	return &Hash{
		Type:         htype,
		Digest:       digest,
		Size:         size,
		PreambleSize: preambleSize,
		ID:           id,
		Expiration:   expiration,
		StorageId:    uint(storageId),
	}, nil
}

func (h *Hash) String() string {
	if h.IsNil() {
		return ""
	}

	if h.s == "" {
		h.s = h.prefix() + base58.Encode(h.DecodedBytes())
	}

	return h.s
}

// DecodedBytes converts this hash to base58-decoded bytes.
func (h *Hash) DecodedBytes() []byte {
	var b []byte
	if !h.IsLive() {
		bufLen := len(h.Digest) + byteutil.LenUvarInt(uint64(h.Size))
		if h.hasStorageId() {
			bufLen += byteutil.LenUvarInt(uint64(h.StorageId))
		}
		if h.Type.Code.IsContent() {
			bufLen += len(h.ID.Bytes())
		} else if h.Type.Code.IsPart() && h.PreambleSize > 0 {
			bufLen += byteutil.LenUvarInt(uint64(h.PreambleSize))
		}

		b = make([]byte, bufLen)
		n := 0

		if h.hasStorageId() {
			n += binary.PutUvarint(b[n:], uint64(h.StorageId))
		}

		n += copy(b[n:], h.Digest)
		n += binary.PutUvarint(b[n:], uint64(h.Size))

		if h.Type.Code.IsContent() {
			n += copy(b[n:], h.ID.Bytes())
		} else if h.Type.Code.IsPart() && h.PreambleSize > 0 {
			n += binary.PutUvarint(b[n:], uint64(h.PreambleSize))
		}
	} else {
		expirationUnix := h.Expiration.Unix()
		bufLen := len(h.Digest)
		if !h.Expiration.IsZero() {
			bufLen += byteutil.LenUvarInt(uint64(expirationUnix))
		}
		if h.hasStorageId() {
			bufLen += byteutil.LenUvarInt(uint64(h.StorageId))
		}

		b = make([]byte, bufLen)
		n := 0

		if h.hasStorageId() {
			n += binary.PutUvarint(b[n:], uint64(h.StorageId))
		}
		if !h.Expiration.IsZero() {
			n += binary.PutUvarint(b[n:], uint64(expirationUnix))
		}

		n += copy(b[n:], h.Digest)
	}
	return b
}

func (h *Hash) IsNil() bool {
	return h == nil || h.Type.Code == UNKNOWN
}

func (h *Hash) IsLive() bool {
	return h != nil && h.Type.Code.IsLive()
}

func (h *Hash) hasStorageId() bool {
	return h != nil && h.Type.Code.hasStorageId()
}

// AssertType checks whether the hash's type equals the provided type
func (h *Hash) AssertType(t Type) error {
	if h.IsNil() || h.Type != t {
		return errors.NoTrace("verify hash", errors.K.Invalid, "reason", "hash type doesn't match",
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
		return errors.NoTrace("verify hash", errors.K.Invalid, "reason", "hash code doesn't match",
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
		return errors.NoTrace("verify hash", errors.K.Invalid, "reason", "hash format doesn't match",
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

func (h *Hash) MarshalCBOR() ([]byte, error) {
	return cbor.Marshal(h.String())
}

func (h *Hash) UnmarshalCBOR(b []byte) error {
	var s string
	err := cbor.Unmarshal(b, &s)
	if err != nil {
		return errors.E("unmarshal hash", err, errors.K.Invalid)
	}
	parsed, err := FromString(s)
	if err != nil {
		return errors.E("unmarshal hash", err, errors.K.Invalid)
	}
	if parsed == nil {
		// empty string parses to nil hash... best we can do is ignore it...
		return nil
	}
	*h = *parsed
	return nil
}

// MarshalText converts this hash to text.
func (h Hash) MarshalText() ([]byte, error) {
	return []byte(h.String()), nil
}

// UnmarshalText parses the hash from the given text.
func (h *Hash) UnmarshalText(text []byte) error {
	parsed, err := FromString(string(text))
	if err != nil {
		return errors.NoTrace("unmarshal hash", err)
	}
	if parsed == nil {
		// empty string parses to nil hash... best we can do is ignore it...
		return nil
	}
	*h = *parsed
	return nil
}

// As returns a copy of this hash with the given code as the type of the new hash.
//
// Deprecated: use AsContentHash() or AsPartHash() instead.
func (h *Hash) As(c Code, id ei.ID) (*Hash, error) {
	e := errors.TemplateNoTrace("convert hash", errors.K.Invalid)
	if h.IsNil() || c == h.Type.Code {
		return h, nil
	} else if h.PreambleSize > 0 {
		return nil, e("reason", "no conversion for parts with preamble", "hash", h)
	} else if h.IsLive() {
		return nil, e("reason", "no conversion for live parts", "hash", h, "code", c)
	} else if _, ok := typeToPrefix[Type{c, h.Type.Format}]; !ok {
		return nil, e("reason", "invalid type", "code", c, "format", h.Type.Format)
	}
	var res = *h // copy
	res.Type.Code = c
	res.s = ""
	if c != Q && c != C {
		res.ID = nil
	} else if id != nil && id.AssertCode(ei.Q) == nil {
		res.ID = id
	} else {
		return nil, e("reason", "invalid id", "id", id)
	}
	res.s = res.String()
	return &res, nil
}

// AsContentHash returns a copy of this (part) hash as a content hash with the given ID. Returns an error if the ID is
// not a valid content ID or if the hash cannot be converted to a content hash.
func (h *Hash) AsContentHash(id ei.ID) (*Hash, error) {
	e := errors.TemplateNoTrace("hash.AsContentHash", errors.K.Invalid, "hash", h, "id", id)
	if err := id.AssertCode(ei.Q); err != nil {
		return nil, e("reason", "invalid content id")
	}
	if h.IsNil() {
		return h, e("reason", "hash is nil")
	}
	if h.Type.Code.IsContent() {
		// replace content id
		var res = *h // copy
		res.ID = id
		res.s = ""
		res.s = res.String()
		if err := res.Validate(); err != nil {
			return nil, e(err)
		}
		return &res, nil
	}
	if h.PreambleSize > 0 {
		return nil, e("reason", "no conversion for parts with preamble")
	}
	if h.IsLive() {
		return nil, e("reason", "no conversion for live parts")
	}

	targetCode := Q
	if h.hasStorageId() {
		targetCode = C
	}

	if _, ok := typeToPrefix[Type{targetCode, h.Type.Format}]; !ok {
		return nil, e("reason", "invalid type", "code", targetCode, "format", h.Type.Format)
	}

	var res = *h // copy
	res.Type.Code = targetCode
	res.ID = id
	res.s = ""
	res.s = res.String()
	if err := res.Validate(); err != nil {
		return nil, e(err)
	}
	return &res, nil
}

// AsPartHash returns a copy of this (content) hash as a part hash, or an error if the hash cannot be converted to a
// part hash.
func (h *Hash) AsPartHash() (*Hash, error) {
	e := errors.TemplateNoTrace("hash.AsPartHash", errors.K.Invalid, "hash", h)
	if h.IsNil() {
		return nil, e("reason", "hash is nil")
	}
	if h.Type.Code.IsPart() {
		return nil, e("reason", "hash is already a part hash")
	}
	if h.Type.Format != Unencrypted {
		return nil, e("reason", "no conversion for encrypted hash")
	}

	targetCode := QPart
	if h.hasStorageId() {
		targetCode = CPart
	}

	if _, ok := typeToPrefix[Type{targetCode, h.Type.Format}]; !ok {
		return nil, e("reason", "invalid type", "code", targetCode, "format", h.Type.Format)
	}

	var res = *h // copy
	res.Type.Code = targetCode
	res.ID = nil
	res.s = ""
	res.s = res.String()
	if err := res.Validate(); err != nil {
		return nil, e(err)
	}
	return &res, nil
}

// Equal returns true if this hash is equal to the provided hash, false otherwise.
func (h *Hash) Equal(h2 *Hash) bool {
	if h == h2 {
		return true
	} else if h == nil || h2 == nil {
		return false
	}
	return h.String() == h2.String()
}

// AssertEqual returns nil if this hash is equal to the provided hash, an error with detailed reason otherwise.
func (h *Hash) AssertEqual(h2 *Hash) error {
	e := errors.TemplateNoTrace("hash.assert-equal", errors.K.Invalid, "expected", h, "actual", h2)
	switch {
	case h == h2:
		return nil
	case h == nil || h2 == nil:
		return e()
	case h.Type != h2.Type:
		return e("reason", "type differs")
	case !bytes.Equal(h.Digest, h2.Digest):
		return e("reason", "digest differs",
			"expected_digest", hex.EncodeToString(h.Digest),
			"actual_digest", hex.EncodeToString(h2.Digest))
	case h.Size != h2.Size:
		return e("reason", "size differs", "expected_size", h.Size, "actual_size", h2.Size)
	case h.PreambleSize != h2.PreambleSize:
		return e("reason", "preamble size differs", "expected_size", h.PreambleSize, "actual_size", h2.PreambleSize)
	case !h.ID.Equal(h2.ID):
		return e("reason", "id differs", "expected_id", h.ID, "actual_id", h2.ID)
	case h.Expiration != h2.Expiration:
		return e("reason", "expiration differs", "expected_expiration", h.Expiration, "actual_expiration", h2.Expiration)
	case h.StorageId != h2.StorageId:
		return e("reason", "storage id differs", "expected_storage_id", h.StorageId, "actual_storage_id", h2.StorageId)
	default:
		return nil
	}
}

func (h *Hash) DigestBytes() []byte {
	if h.IsNil() {
		return nil
	}
	return h.Digest
}

func (h *Hash) Describe() string {
	sb := strings.Builder{}

	add := func(s string) {
		sb.WriteString(s)
		sb.WriteString("\n")
	}

	add("type:          " + h.Type.Describe())
	sc := "0 (default)"
	if h.hasStorageId() {
		sc = strconv.FormatUint(uint64(h.StorageId), 10)
	}
	add("storage id:    " + sc)
	add("digest:        0x" + hex.EncodeToString(h.Digest))
	if !h.IsLive() {
		add("size:          " + strconv.FormatInt(h.Size, 10))
		if h.PreambleSize > 0 {
			add("preamble_size: " + strconv.FormatInt(h.PreambleSize, 10))
		}
	} else {
		add("expiration:    " + h.Expiration.String())
	}
	if h.Type.Code == Q || h.Type.Code == C {
		add("qid:           " + h.ID.String())
		qphash, err := h.AsPartHash()
		if err == nil {
			add("part:          " + qphash.String())
		} else {
			add("part:          Failed to convert to part hash: " + err.Error())
		}
	}

	return sb.String()
}

func (h *Hash) IsValid() bool {
	return h.Validate() == nil
}

func (h *Hash) Validate() error {
	e := errors.TemplateNoTrace("hash.Validate", errors.K.Invalid.Default(), "hash", h)
	if h == nil {
		return e("reason", "hash is nil")
	}
	if !h.Type.IsValid() {
		return e("reason", "invalid type", "code", h.Type.Code, "format", h.Type.Format)
	}
	if len(h.Digest) != sha256.Size {
		if len(h.Digest) == 0 {
			return e("reason", "empty digest")
		} else if !h.IsLive() {
			return e("reason", "invalid digest", "size", len(h.Digest))
		}
	}
	if h.Size < 0 {
		return e("reason", "invalid part size", "size", h.Size)
	}
	if h.PreambleSize < 0 {
		return e("reason", "invalid preamble size", "preamble_size", h.Size)
	}
	if !h.Type.Code.hasStorageId() && h.StorageId != 0 {
		return e("reason", "storage id not allowed", "storage_id", h.StorageId)
	}

	if h.Type.Code.IsContent() {
		if err := h.ID.AssertCode(ei.Q); err != nil {
			return e("reason", "invalid id", "id", h.ID)
		}
		if !h.Expiration.IsZero() {
			return e("reason", "expiration not allowed for content hash", "expiration", h.Expiration)
		}
		if h.PreambleSize != 0 {
			return e("reason", "preamble size not allowed for content hash", "preamble_size", h.Size)
		}
	} else if h.Type.Code.IsPart() {
		if h.ID.IsValid() {
			return e("reason", "id not allowed for part hash", "id", h.ID)
		}
		if h.Type.Code.IsLive() {
			if h.PreambleSize != 0 {
				return e("reason", "preamble size not allowed for live part hash", "preamble_size", h.Size)
			}
		} else {
			if !h.Expiration.IsZero() {
				return e("reason", "expiration not allowed for non-live hash", "expiration", h.Expiration)
			}
		}
	}
	return nil
}
