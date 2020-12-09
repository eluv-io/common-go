package eat

import (
	"github.com/qluvio/content-fabric/errors"
)

// Formats defines the available encoding formats for auth tokens
const Formats enumFormat = 0

type TokenFormat = *tokenFormat

var defaultFormat = Formats.JsonCompressed()

type tokenFormat struct {
	Prefix string
	Name   string
}

func (f *tokenFormat) String() string {
	return f.Name
}

func (f *tokenFormat) Validate() error {
	e := errors.Template("validate token format", errors.K.Invalid)
	if f == nil {
		return e("reason", "format is nil")
	}
	if f == Formats.Unknown() {
		return e("format", f.Name)
	}
	return nil
}

var allFormats = []*tokenFormat{
	{"nk", "unknown"},         // 0
	{"__", "legacy"},          // 1
	{"j_", "json"},            // 2
	{"jc", "json-compressed"}, // 3
	{"c_", "cbor"},            // 4
	{"cc", "cbor-compressed"}, // 5
	{"b_", "custom"},          // 6
}

type enumFormat int

func (enumFormat) Unknown() TokenFormat        { return allFormats[0] }
func (enumFormat) Legacy() TokenFormat         { return allFormats[1] } // base64(JSON)
func (enumFormat) Json() TokenFormat           { return allFormats[2] } // prefix+base58(JSON)
func (enumFormat) JsonCompressed() TokenFormat { return allFormats[3] } // prefix+base58(deflate(JSON))
func (enumFormat) Cbor() TokenFormat           { return allFormats[4] } // prefix+base58(CBOR)
func (enumFormat) CborCompressed() TokenFormat { return allFormats[5] } // prefix+base58(deflate(CBOR))
func (enumFormat) Custom() TokenFormat         { return allFormats[6] } // prefix+base58(binary-custom)

var prefixToFormat = map[string]*tokenFormat{}

func init() {
	for _, f := range allFormats {
		prefixToFormat[f.Prefix] = f
		if len(f.Prefix) != 2 {
			panic(errors.E("invalid format prefix definition",
				"format", f.Name,
				"prefix", f.Prefix))
		}
	}
}
