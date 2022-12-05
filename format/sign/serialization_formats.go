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

func (f *serializationFormat) Validate() error {
	e := errors.Template("validate serialization format", errors.K.Invalid)
	if f == nil {
		return e("reason", "format is nil")
	}
	if f == SerializationFormats.Unknown() {
		return e("format", f.Name)
	}
	return nil
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
