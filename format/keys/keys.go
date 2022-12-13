package keys

import (
	"bytes"

	"github.com/mr-tron/base58/base58"

	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
)

// Code is the type of a Key
type Code uint8

// KeyCode is the legacy alias of Code
// @deprecated use Code instead
type KeyCode = Code

func (c Code) String() string {
	return codeToPrefix[c]
}

// FromString parses the given key string and returns the Key. Returns an error
// if the string is not a Key or a Key of the wrong type.
func (c Code) FromString(s string) (Key, error) {
	key, err := FromString(s)
	if err != nil {
		return nil, err
	}
	return key, key.AssertCode(c)
}

// KeyLen returns the expected length of a key for the given code. Returns -1 if unknown or not constant.
func (c Code) KeyLen() int {
	switch c {
	case ES256KSecretKey:
		return 32
	case ES256KPublicKey:
		return 33
	case ED25519SecretKey:
		return 64
	case ED25519PublicKey:
		return 32
	case SR25519SecretKey:
		return 32
	case SR25519PublicKey:
		return 32
	case BLS12381SecretKey:
		return 32
	case BLS12381PublicKey:
		return 48
	default:
		return -1
	}
}

// lint disable
const (
	UNKNOWN             Code = iota
	Primary                  // code of Primary key
	ReEncryption             // @deprecated
	Delegate                 // @deprecated
	TargetReEncryption       // code of re-encryption key
	Decryption               // code of decryption key
	SymmetricKey             // primary key: symmkey
	PrimSecretKey            // primary key: secret key
	PrimPublicKey            // primary key: public key
	RekEncKeyBytes           // re-encryption key: key bytes
	TgtSecretKey             // re-encryption key: secret key
	TgtPublicKey             // re-encryption key: public key
	EthPublicKey             // @deprecated use ES256KPublicKey
	EthPrivateKey            // @deprecated use ECDSAPrivateKey
	FabricNodePublicKey      // @deprecated use ES256KPublicKey
	UserPublicKey            // @deprecated use ES256KPublicKey
	ES256KSecretKey          // secret key for generating Ethereum ECDSA signatures - see sign.ES256K
	ES256KPublicKey          // public key for validating Ethereum ECDSA signatures - see sign.ES256K
	ED25519SecretKey         // secret key for generating ED25519 signatures - see sign.ED25519
	ED25519PublicKey         // public key for validating ED25519 signatures - see sign.ED25519
	SR25519SecretKey         // secret key for generating Schnorr signatures - see sign.SR25519
	SR25519PublicKey         // public key for validating Schnorr signatures - see sign.SR25519
	BLS12381SecretKey        // secret key for elliptic curve BLS12-381
	BLS12381PublicKey        // public key for elliptic curve BLS12-381

	// KUNKNOWN is the legacy alias for UNKNOWN
	// @deprecated use UNKNOWN instead
	KUNKNOWN = UNKNOWN
)

const codeLen = 1
const prefixLen = 4

var codeToPrefix = map[Code]string{}
var prefixToCode = map[string]Code{
	"kunk": UNKNOWN,
	"kp__": Primary,
	"kre_": ReEncryption,
	"kde_": Delegate,
	"ktre": TargetReEncryption,
	"kdec": Decryption,
	"kpsy": SymmetricKey,
	"kpsk": PrimSecretKey,
	"kppk": PrimPublicKey,
	"kreb": RekEncKeyBytes,
	"ktsk": TgtSecretKey,
	"ktpk": TgtPublicKey,

	"kepk": EthPublicKey,
	"kesk": EthPrivateKey,
	"knod": FabricNodePublicKey,
	"kupk": UserPublicKey,

	"ksec": ES256KSecretKey,
	"kpec": ES256KPublicKey,
	"ksed": ED25519SecretKey,
	"kped": ED25519PublicKey,
	"kssr": SR25519SecretKey,
	"kpsr": SR25519PublicKey,
	"ksbl": BLS12381SecretKey,
	"kpbl": BLS12381PublicKey,
}

func init() {
	for prefix, code := range prefixToCode {
		if len(prefix) != prefixLen {
			log.Fatal("invalid Key prefix definition", "prefix", prefix)
		}
		codeToPrefix[code] = prefix
	}
}

// Key is the type representing a cryptographic Key. Keys follow the multiformat principle and are prefixed with their
// type (a varint). Unlike other multiformat implementations like multihash, the type is serialized to textual form
// (String(), JSON) as a short text prefix instead of their encoded varint for increased readability.
type Key []byte

// KID is the legacy alias for Key.
// @deprecated use Key instead.
type KID = Key

func (k Key) String() string {
	if len(k) <= codeLen {
		return ""
	}
	return k.prefix() + base58.Encode(k[codeLen:])
}

// AssertCode checks whether the Key's Code equals the provided Code
func (k Key) AssertCode(c Code) error {
	if len(k) < codeLen || k.Code() != c {
		return errors.E("AssertCode", errors.K.Invalid,
			"expected", codeToPrefix[c],
			"actual", k.prefix())
	}
	return nil
}

func (k Key) prefix() string {
	p, found := codeToPrefix[k.Code()]
	if !found {
		return codeToPrefix[UNKNOWN]
	}
	return p
}

func (k Key) Code() Code {
	if k.IsNil() {
		return UNKNOWN
	}
	return Code(k[0])
}

// MarshalText implements custom marshaling using the string representation.
func (k Key) MarshalText() ([]byte, error) {
	return []byte(k.String()), nil
}

func (k Key) Bytes() []byte {
	if len(k) < 1 {
		return nil
	}
	return k[1:]
}

// UnmarshalText implements custom unmarshaling from the string representation.
func (k *Key) UnmarshalText(text []byte) error {
	parsed, err := KFromString(string(text))
	if err != nil {
		return errors.E("unmarshal Key", errors.K.Invalid, err)
	}
	*k = parsed
	return nil
}

func (k Key) Is(s string) bool {
	sID, err := KFromString(s)
	if err != nil {
		return false
	}
	return bytes.Equal(k, sID)
}

func (k Key) Validate() error {
	e := errors.TemplateNoTrace("Validate", errors.K.Invalid)

	if len(k) <= codeLen {
		return e("reason", "key empty")
	}

	l := len(k) - codeLen
	expectedLen := k.Code().KeyLen()
	if expectedLen != -1 && l != expectedLen {
		return e("reason", "invalid key length", "expected", expectedLen, "actual", l)
	}

	return nil
}

func (k Key) IsNil() bool {
	return len(k) == 0
}

func (k Key) IsValid() bool {
	return k.Validate() == nil
}

// New creates a new Key from the given code and key material.
func New(code Code, key []byte) Key {
	return Key(append([]byte{byte(code)}, key...))
}

// NewKID creates a new key - retained for backwards compatibility.
// @deprecated - use New()
func NewKID(code Code, key []byte) Key {
	return New(code, key)
}

// KFromString parses a Key from the given string representation.
// @deprecated use FromString() instead
func KFromString(s string) (Key, error) {
	return FromString(s)
}

// FromString parses a Key from the given string representation.
func FromString(s string) (Key, error) {
	if len(s) == 0 {
		return nil, nil
	}

	e := errors.Template("parse key", errors.K.Invalid.Default(), "key", s)
	if len(s) <= prefixLen {
		return nil, e("reason", "empty key")
	}

	code, found := prefixToCode[s[:prefixLen]]
	if !found {
		return nil, e("reason", "unknown prefix")
	}

	dec, err := base58.Decode(s[prefixLen:])
	if err != nil {
		return nil, e(err, "reason", "invalid encoding")
	}

	key := New(code, dec)
	if err = key.Validate(); err != nil {
		return nil, e(err)
	}

	return key, nil
}

// Parse parses a Key from the given string representation.
func Parse(s string) (Key, error) {
	return FromString(s)
}

// MustParse is like Parse, but panics in case of errors.
func MustParse(s string) Key {
	key, err := FromString(s)
	if err != nil {
		panic(err)
	}
	return key
}
