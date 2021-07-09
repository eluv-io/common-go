package encryption

import (
	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/hash"
)

// Scheme is the encryption scheme of a resource.
type Scheme uint

const (
	UNKNOWN   Scheme = iota
	None             // Unencrypted
	ClientGen        // Encrypted, client-generated content key
)

// Schemes lists all schemes - including UNKNOWN, which is used in filename generation for parts
// => see aferoFactory.Create and aferoFactory.OpenWriter
var Schemes = []Scheme{UNKNOWN, None, ClientGen}

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
