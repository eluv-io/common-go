package encryption

import (
	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/errors-go"
)

// Scheme is the encryption scheme of a resource.
// Byte type is used so that scheme can be stored directly as a byte, e.g. in tokens
type Scheme byte

const ( // List should be preserved and not re-arranged or removed from, only added to
	UNKNOWN   Scheme = iota
	None             // Unencrypted
	ClientGen        // Encrypted, client-generated content key
)

// Schemes lists all schemes - including UNKNOWN, which is used in filename generation for parts
// => see aferoFactory.Create and aferoFactory.OpenWriter
var Schemes = map[Scheme]bool{
	UNKNOWN:   true,
	None:      true,
	ClientGen: true,
}

var schemeToName = map[Scheme]string{
	UNKNOWN:   "",
	None:      "none",
	ClientGen: "cgck", //NOTE: 'cgck' scheme means encryption keys used with clear block of 1000000 bytes
}
var nameToScheme = map[string]Scheme{}

var schemeToFormat = map[Scheme]hash.Format{
	UNKNOWN:   hash.Unencrypted,
	None:      hash.Unencrypted,
	ClientGen: hash.AES128AFGH,
}

// In the case of multiple schemes mapping to a single format, the last scheme is used for formatToScheme
var formatToScheme = map[hash.Format]Scheme{}

func init() {
	for scheme, name := range schemeToName {
		nameToScheme[name] = scheme
	}
	for scheme, format := range schemeToFormat {
		formatToScheme[format] = scheme
	}
}

func FromString(str string) (Scheme, error) {
	s, ok := nameToScheme[str]
	if !ok {
		return 0, errors.E("parse scheme", errors.K.Invalid, "reason", "invalid scheme", "scheme", str)
	}
	return s, nil
}

func FromHashFormat(format hash.Format) (Scheme, error) {
	s, ok := formatToScheme[format]
	if !ok {
		return 0, errors.E("parse scheme", errors.K.Invalid, "reason", "invalid hash_format", "hash_format", format)
	}
	return s, nil
}

func (s Scheme) String() string {
	return schemeToName[s]
}

func (s Scheme) HashFormat() hash.Format {
	return schemeToFormat[s]
}

func (s Scheme) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

func (s *Scheme) UnmarshalText(text []byte) error {
	var err error
	*s, err = FromString(string(text))
	return err
}
