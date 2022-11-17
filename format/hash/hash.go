package hash

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
	"github.com/mr-tron/base58/base58"

	ei "github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/preamble"
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
)

func (c Code) IsLive() bool {
	return c == QPartLive || c == QPartLiveTransient
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

// Format is the format of a hash
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
	}
	switch t.Format {
	case Unencrypted:
		f = "unencrypted"
	case AES128AFGH:
		f = "encrypted with AES-128, AFGHG BLS12-381, 1 MB block size"
	}
	return c + ", " + f
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
	"hqt_": Type{QPartLiveTransient, Unencrypted},
	"hqte": Type{QPartLiveTransient, AES128AFGH},
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
//	Q format : type (1 byte) | digest (var bytes) | size (var bytes) | id (var bytes)
//	QPart format : type (1 byte) | digest (var bytes) | size (var bytes) | preamble_size (var bytes, optional)
//	QPartLive format : type (1 byte) | expiration (var bytes) | digest (var bytes)
//	QPartLiveTransient format: type (1 byte) | expiration (var bytes) | digest (var bytes)
//  (Deprecated) QPartLive format : type (1 byte) | digest (24-25 bytes)
type Hash struct {
	Type         Type
	Digest       []byte
	Size         int64
	PreambleSize int64
	ID           ei.ID
	Expiration   utc.UTC
	s            string
}

// NewObject creates a new object hash with the given type, digest, size, and ID
func NewObject(htype Type, digest []byte, size int64, id ei.ID) (*Hash, error) {
	e := errors.Template("init hash", errors.K.Invalid)
	if _, ok := typeToPrefix[htype]; !ok {
		return nil, e("reason", "invalid type", "code", htype.Code, "format", htype.Format)
	} else if htype.Code == UNKNOWN {
		return nil, nil
	} else if htype.Code != Q {
		return nil, e("reason", "code not supported", "code", htype.Code)
	}

	if len(digest) != sha256.Size {
		return nil, e("reason", "invalid digest", "digest", digest)
	}

	if size < 0 {
		return nil, e("reason", "invalid size", "size", size)
	}

	if id.AssertCode(ei.Q) != nil {
		return nil, e("reason", "invalid id", "id", id)
	}

	h := &Hash{Type: htype, Digest: digest, Size: size, ID: id}
	h.s = h.String()

	return h, nil
}

// NewPart creates a new non-live part hash with the given type, digest, size, and optional preamble size
func NewPart(htype Type, digest []byte, size int64, preambleSize int64) (*Hash, error) {
	e := errors.Template("init hash", errors.K.Invalid)
	if _, ok := typeToPrefix[htype]; !ok {
		return nil, e("reason", "invalid type", "code", htype.Code, "format", htype.Format)
	} else if htype.Code == UNKNOWN {
		return nil, nil
	} else if htype.Code != QPart {
		return nil, e("reason", "code not supported", "code", htype.Code)
	}

	if len(digest) != sha256.Size {
		return nil, e("reason", "invalid digest", "digest", digest)
	}

	if size < 0 {
		return nil, e("reason", "invalid size", "size", size)
	}

	if preambleSize < 0 {
		return nil, e("reason", "invalid preamble size", "preamble_size", size)
	}

	h := &Hash{Type: htype, Digest: digest, Size: size, PreambleSize: preambleSize}
	h.s = h.String()

	return h, nil
}

// NewLive creates a new live hash with the given type, digest, and expiration
func NewLive(htype Type, digest []byte, expiration utc.UTC) (*Hash, error) {
	e := errors.Template("init hash", errors.K.Invalid)
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
	if s == "" {
		return nil, nil
	}
	e := errors.Template("parse hash", errors.K.Invalid, "string", s)

	if len(s) <= prefixLen {
		return nil, e("reason", "invalid token string")
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
	var size, preambleSize int64
	var id ei.ID
	var expiration utc.UTC
	if !htype.Code.IsLive() {
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

		if htype.Code == QPart {
			sz, m = binary.Uvarint(b[n:])
			if m < 0 {
				return nil, errors.E("parse hash", errors.K.Invalid, "reason", "invalid preamble size", "string", s)
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
		e, m := binary.Uvarint(b[n:])
		if m <= 0 {
			return nil, errors.E("parse hash", errors.K.Invalid, "reason", "invalid expiration", "string", s)
		}
		expiration = utc.Unix(int64(e), 0)
		n += m

		// Parse digest
		digest = b[n:]
	}

	return &Hash{Type: htype, Digest: digest, Size: size, PreambleSize: preambleSize, ID: id, Expiration: expiration, s: s}, nil
}

func (h *Hash) String() string {
	if h.IsNil() {
		return ""
	}

	if h.s == "" && len(h.Digest) > 0 {
		var b []byte
		if !h.IsLive() {
			b = make([]byte, len(h.Digest))

			copy(b, h.Digest)

			s := make([]byte, binary.MaxVarintLen64)
			n := binary.PutUvarint(s, uint64(h.Size))
			b = append(b, s[:n]...)

			if h.Type.Code == QPart && h.PreambleSize > 0 {
				n := binary.PutUvarint(s, uint64(h.PreambleSize))
				b = append(b, s[:n]...)
			} else if h.Type.Code == Q && h.ID.IsValid() {
				b = append(b, h.ID.Bytes()...)
			}
		} else {
			b = make([]byte, binary.MaxVarintLen64)

			var n int
			if !h.Expiration.IsZero() {
				n = binary.PutUvarint(b, uint64(h.Expiration.Unix()))
			}

			b = append(b[:n], h.Digest...)
		}
		h.s = h.prefix() + base58.Encode(b)
	}

	return h.s
}

func (h *Hash) IsNil() bool {
	return h == nil || h.Type.Code == UNKNOWN
}

func (h *Hash) IsLive() bool {
	return h != nil && h.Type.Code.IsLive()
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

// As returns a copy of this hash with the given code as the type of the new hash.
func (h *Hash) As(c Code, id ei.ID) (*Hash, error) {
	if h.IsNil() || c == h.Type.Code {
		return h, nil
	} else if h.PreambleSize > 0 {
		return nil, errors.E("convert hash", errors.K.Invalid, "reason", "no conversion for parts with preamble", "hash", h)
	} else if h.IsLive() || c.IsLive() {
		return nil, errors.E("convert hash", errors.K.Invalid, "reason", "no conversion for live parts", "hash", h, "code", c)
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
	e := errors.Template("hash.assert-equal", errors.K.Invalid, "expected", h, "actual", h2)
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
	add("digest:        0x" + hex.EncodeToString(h.Digest))
	if !h.IsLive() {
		add("size:          " + strconv.FormatInt(h.Size, 10))
		if h.PreambleSize > 0 {
			add("preamble_size: " + strconv.FormatInt(h.PreambleSize, 10))
		}
	} else {
		add("expiration:    " + h.Expiration.String())
	}
	if h.Type.Code == Q {
		add("qid:           " + h.ID.String())
		qphash, err := h.As(QPart, nil)
		if err == nil {
			add("part:          " + qphash.String())
		}
	}

	return sb.String()
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Digest encapsulates a message digest function which produces a specific type of Hash
type Digest struct {
	hash.Hash
	preamble *preamble.Sizer
	htype    Type
	id       ei.ID
	size     int64
	psize    int64
}

// make sure Digest implements the Hash interface
var _ hash.Hash = (*Digest)(nil)

// NewDigest creates a new digest. Does not support live part hashes
func NewDigest(h hash.Hash, t Type) *Digest {
	return &Digest{Hash: h, preamble: preamble.NewSizer(), htype: t}
}

func (d *Digest) WithPreamble(preambleSize int64) *Digest {
	if d.htype.Code == QPart {
		if preambleSize > 0 {
			d.psize = preambleSize
		} else {
			// Calculate preamble size
			var err error
			d.psize, err = d.preamble.Size()
			if err != nil {
				// Should not happen
				log.Warn("invalid hash", "error", err)
			}
		}
	} else {
		// Should not happen
		log.Warn("invalid hash", "error", "preamble not applicable", "code", d.htype.Code)
	}
	return d
}

func (d *Digest) WithID(i ei.ID) *Digest {
	if d.htype.Code == Q {
		d.id = i
	}
	return d
}

func (d *Digest) Write(p []byte) (int, error) {
	n, err := d.Hash.Write(p)
	if err == nil && d.htype.Code == QPart {
		n2, err2 := d.preamble.Write(p)
		if err2 != nil || n2 != n {
			// Should not happen
			log.Warn("invalid hash", "error", err, "n", n, "n2", n2)
		}
	}
	d.size += int64(n)
	return n, err
}

// AsHash finalizes the digest calculation using all the bytes that were previously written to this digest object and
// return the result as a Hash.
func (d *Digest) AsHash() *Hash {
	b := d.Hash.Sum(nil)
	var h *Hash
	var err error
	if d.htype.Code == Q {
		h, err = NewObject(d.htype, b, d.size, d.id)
	} else {
		h, err = NewPart(d.htype, b, d.size, d.psize)
	}
	if err != nil {
		// errors must be caught by unit tests!
		log.Fatal("invalid hash", "error", err)
	}
	return h
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func CalcHash(reader io.ReadSeeker, size ...int64) (*Hash, error) {
	digest := NewDigest(sha256.New(), Type{QPart, Unencrypted})

	// Check for preamble
	var preambleSize int64
	var err error
	if len(size) > 0 {
		_, _, preambleSize, err = preamble.Read(reader, false, size[0])
	} else {
		_, _, preambleSize, err = preamble.Read(reader, false)
	}
	if errors.IsNotExist(err) {
		preambleSize = 0
	} else if err != nil {
		return nil, err
	}

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

	if preambleSize > 0 {
		digest = digest.WithPreamble(preambleSize)
	}

	return digest.AsHash(), nil
}
