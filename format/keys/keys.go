package keys

import (
	"bytes"

	"github.com/mr-tron/base58/base58"

	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
)

// KeyCode is the type of an ID
type KeyCode uint8

// FromString parses the given string and returns the ID. Returns an error
// if the string is not a ID or an ID of the wrong type.
func (c KeyCode) FromString(s string) (KID, error) {
	id, err := KFromString(s)
	if err != nil {
		return nil, err
	}
	return id, id.AssertCode(c)
}

// lint disable
const (
	KUNKNOWN            KeyCode = iota
	Primary                     // code of Primary key
	ReEncryption                // @deprecated
	Delegate                    // @deprecated
	TargetReEncryption          // code of re-encryption key
	Decryption                  // code of decryption key
	SymmetricKey                // primary key: symmkey
	PrimSecretKey               // primary key: secret key
	PrimPublicKey               // primary key: public key
	RekEncKeyBytes              // re-encryption key: key bytes
	TgtSecretKey                // re-encryption key: secret key
	TgtPublicKey                // re-encryption key: public key
	EthPublicKey                // @deprecated use ES256KPublicKey
	EthPrivateKey               // @deprecated use ECDSAPrivateKey
	FabricNodePublicKey         // @deprecated use ES256KPublicKey
	UserPublicKey               // @deprecated use ES256KPublicKey
	ES256KSecretKey             // secret key for generating Ethereum ECDSA signatures - see sign.ES256K
	ES256KPublicKey             // public key for validating Ethereum ECDSA signatures - see sign.ES256K
	ED25519SecretKey            // secret key for generating ED25519 signatures - see sign.ED25519
	ED25519PublicKey            // public key for validating ED25519 signatures - see sign.ED25519
	SR25519SecretKey            // secret key for generating Schnorr signatures - see sign.SR25519
	SR25519PublicKey            // public key for validating Schnorr signatures - see sign.SR25519
)

const codeLen = 1
const prefixLen = 4

var keyCodeToPrefix = map[KeyCode]string{}
var keyPrefixToCode = map[string]KeyCode{
	"kunk": KUNKNOWN,
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
}

func init() {
	for prefix, code := range keyPrefixToCode {
		if len(prefix) != prefixLen {
			log.Fatal("invalid Key ID prefix definition", "prefix", prefix)
		}
		keyCodeToPrefix[code] = prefix
	}
}

// KID is the type representing a Key ID. IDs follow the multiformat principle
// and are prefixed with their type (a varint). Unlike other multiformat
// implementations like multihash, the type is serialized to textual form
// (String(), JSON) as a short text prefix instead of their encoded varint for
// increased readability.
type KID []byte

// Key is an alias for KID. KID is somehow misleading since the bytes are the actual key, not just some identifier of a
// key...
type Key = KID

func (id KID) String() string {
	if len(id) <= codeLen {
		return ""
	}
	return id.prefix() + base58.Encode(id[codeLen:])
}

// AssertCode checks whether the ID's Code equals the provided Code
func (id KID) AssertCode(c KeyCode) error {
	if len(id) < codeLen || id.Code() != c {
		return errors.E("ID Code check", errors.K.Invalid,
			"expected", keyCodeToPrefix[c],
			"actual", id.prefix())
	}
	return nil
}

func (id KID) prefix() string {
	p, found := keyCodeToPrefix[id.Code()]
	if !found {
		return keyCodeToPrefix[KUNKNOWN]
	}
	return p
}

func (id KID) Code() KeyCode {
	return KeyCode(id[0])
}

// MarshalText implements custom marshaling using the string representation.
func (id KID) MarshalText() ([]byte, error) {
	return []byte(id.String()), nil
}

func (id KID) Bytes() []byte {
	if len(id) < 1 {
		return nil
	}
	return id[1:]
}

// UnmarshalText implements custom unmarshaling from the string representation.
func (id *KID) UnmarshalText(text []byte) error {
	parsed, err := KFromString(string(text))
	if err != nil {
		return errors.E("unmarshal KID", errors.K.Invalid, err)
	}
	*id = parsed
	return nil
}

func (id KID) Is(s string) bool {
	sID, err := KFromString(s)
	if err != nil {
		return false
	}
	return bytes.Equal(id, sID)
}

func (id KID) IsValid() bool {
	return len(id) > codeLen
}

func NewKID(code KeyCode, codeBytes []byte) KID {
	return KID(append([]byte{byte(code)}, codeBytes...))
}

// KFromString parses an KID from the given string representation.
func KFromString(s string) (KID, error) {
	if len(s) == 0 {
		return nil, nil
	}
	if len(s) <= prefixLen {
		return nil, errors.E("parse KID", errors.K.Invalid).With("string", s)
	}

	code, found := keyPrefixToCode[s[:prefixLen]]
	if !found {
		return nil, errors.E("parse KID", errors.K.Invalid, "reason", "unknown prefix", "string", s)
	}

	dec, err := base58.Decode(s[prefixLen:])
	if err != nil {
		return nil, errors.E("parse KID", errors.K.Invalid, err, "string", s)
	}
	b := []byte{byte(code)}
	return KID(append(b, dec...)), nil
}
