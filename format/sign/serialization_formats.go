package sign

import (
	"github.com/eluv-io/errors-go"
)

// SerializationFormats defines the available serialization formats for signed messages
const SerializationFormats enumSerializationFormat = 0

type SerializationFormat = *serializationFormat

type serializationFormat struct {
	Prefix string
	Name   string
}

func (f *serializationFormat) String() string {
	return f.Name
}

func (f *serializationFormat) MarshalText() ([]byte, error) {
	return []byte(f.String()), nil
}

func (f *serializationFormat) UnmarshalText(data []byte) error {
	s := string(data)
	for _, sf := range allSerializationFormats {
		if sf.Name == s || sf.Prefix == s {
			*f = *sf
			return nil
		}
	}
	*f = *SerializationFormats.Unknown()
	return nil
}

func (f *serializationFormat) Validate() error {
	e := errors.Template("validate serialization format", errors.K.Invalid)
	if f == nil {
		return e("reason", "format is nil")
	}
	if *f == *SerializationFormats.Unknown() {
		return e("format", f.Name)
	}
	return nil
}

func (f *serializationFormat) Unknown() bool {
	if f == nil {
		return true
	}
	return *f == *SerializationFormats.Unknown()
}

func (f *serializationFormat) Scale() bool {
	if f == nil {
		return false
	}
	return *f == *SerializationFormats.Scale()
}

func (f *serializationFormat) EthKeccak() bool {
	if f == nil {
		return false
	}
	return *f == *SerializationFormats.EthKeccak()
}

var allSerializationFormats = []*serializationFormat{
	{"uk", "unknown"},    // 0
	{"sc", "scale"},      // 1
	{"ek", "eth_keccak"}, // 2
}

type enumSerializationFormat int

func (enumSerializationFormat) Unknown() SerializationFormat   { return allSerializationFormats[0] }
func (enumSerializationFormat) Scale() SerializationFormat     { return allSerializationFormats[1] } // SCALE encoding
func (enumSerializationFormat) EthKeccak() SerializationFormat { return allSerializationFormats[2] } // Ethereum Keccak

var prefixToSerializationFormat = map[string]*serializationFormat{}

func init() {
	for _, f := range allSerializationFormats {
		prefixToSerializationFormat[f.Prefix] = f
		if len(f.Prefix) != 2 {
			panic(errors.E("invalid format prefix definition",
				"format", f.Name,
				"prefix", f.Prefix))
		}
	}
}
