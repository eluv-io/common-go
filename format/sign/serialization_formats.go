package sign

import (
	"github.com/eluv-io/errors-go"
)

// SerializationFormats defines the available serialization formats for signed messages
const SerializationFormats enumSerializationFormat = 0

type SerializationFormat string

//goland:noinspection GoMixedReceiverTypes
func (f SerializationFormat) Validate() error {
	e := errors.Template("validate serialization format",
		"format", f,
		errors.K.Invalid)
	if f == "" {
		return e("reason", "format is empty")
	}
	if f.Unknown() {
		return e("reason", "format is unknown")
	}
	return nil
}

//goland:noinspection GoMixedReceiverTypes
func (f *SerializationFormat) UnmarshalText(data []byte) error {
	s := string(data)
	for _, sf := range allSerializationFormats {
		if string(sf) == s {
			*f = sf
			return nil
		}
	}
	*f = SerializationFormats.Unknown()
	// PENDING(GIL): not sure what's best
	return nil //errors.NoTrace("invalid format", errors.K.Invalid, "format", s)
}

//goland:noinspection GoMixedReceiverTypes
func (f SerializationFormat) Unknown() bool {
	for i := 1; i < len(allSerializationFormats); i++ {
		if f == allSerializationFormats[i] {
			return false
		}
	}
	return true
}

//goland:noinspection GoMixedReceiverTypes
func (f SerializationFormat) Scale() bool {
	return f == SerializationFormats.Scale()
}

//goland:noinspection GoMixedReceiverTypes
func (f SerializationFormat) EthKeccak() bool {
	return f == SerializationFormats.EthKeccak()
}

var allSerializationFormats = []SerializationFormat{
	"unknown",    // 0
	"scale",      // 1
	"eth_keccak", // 2
}

type enumSerializationFormat int

func (enumSerializationFormat) Unknown() SerializationFormat   { return allSerializationFormats[0] }
func (enumSerializationFormat) Scale() SerializationFormat     { return allSerializationFormats[1] } // SCALE encoding
func (enumSerializationFormat) EthKeccak() SerializationFormat { return allSerializationFormats[2] } // Ethereum Keccak
