package id

import (
	"encoding/binary"
	"encoding/hex"
	"strings"

	"github.com/eluv-io/common-go/util/byteutil"
	"github.com/eluv-io/errors-go"
)

// Embed embeds the given ID into the primary ID. Currently supports embedding a tenant id `id.Tenant` in a content ID
// `id.Q` or a library ID `id.QLib`, resulting in a composed ID with code `id.TQ` and `id.TLib` respectively. Any other
// combination of IDs results in an invalid Composed struct, which returns false as result of a call to IsValid() and in
// which the primary or tenant ID or both are nil.
func Embed(primary ID, embed ID) Composed {
	return Compose(composedCode(primary, embed), primary.Bytes(), embed.Bytes())
}

// Compose creates a composed ID of the given code from the bytes of the given primary and embed IDs. Empty byte slices
// for the primary/embed IDs result in an invalid Composed struct, which returns false as result of a call to IsValid()
// and in which the primary or tenant ID or both are nil.
func Compose(code Code, primary []byte, embed []byte) Composed {
	codeEmbedded := embeddedCode(code)
	if codeEmbedded == UNKNOWN || len(embed) == 0 {
		if len(primary) == 0 {
			return Composed{} // invalid
		}
		full := NewID(code, primary)
		return Composed{
			full:     full,
			primary:  full,
			embedded: nil,
			strCache: full.String(),
		}
	}

	if len(primary) == 0 {
		// return a composed that has only the embedded ID set - important when creating composed IDs where for example
		// the qid is nil, but the tenant id needs to be "transported" with the composed ID.
		return Composed{
			full:     nil,
			primary:  nil,
			embedded: NewID(codeEmbedded, embed),
			strCache: "",
		}
	}

	embedSize := len(embed)
	viSize := byteutil.LenUvarInt(uint64(embedSize))
	bts := make([]byte, 1+viSize+embedSize+len(primary))

	bts[0] = byte(code)
	off := 1
	off += binary.PutUvarint(bts[off:], uint64(embedSize))
	off += copy(bts[off:], embed)
	off += copy(bts[off:], primary)
	full := ID(bts)

	return Composed{
		full:     full,
		primary:  NewID(primaryCode(code), primary),
		embedded: NewID(codeEmbedded, embed),
		strCache: full.String(),
	}
}

// Decompose extracts the embedded ID found in the given ID. If there is no embedded ID, the resulting Composed struct
// will return nil for Embedded().
func Decompose(id ID) Composed {
	var res = Composed{
		full:     id,
		primary:  id,
		strCache: id.String(),
	}

	codeEmbedded := embeddedCode(id.Code())
	if codeEmbedded == UNKNOWN {
		return res
	}

	bts := id.Bytes()

	// size of the embedded ID
	sz, m := binary.Uvarint(bts)
	if m <= 0 || len(bts) < m+1 {
		return res
	}
	bts = bts[m:]

	res.embedded = make([]byte, sz+1)
	res.embedded[0] = byte(codeEmbedded)
	copy(res.embedded[1:], bts[:sz])
	bts = bts[sz:]

	res.primary = NewID(primaryCode(id.Code()), bts)
	return res
}

func embeddedCode(code Code) Code {
	switch code {
	case TQ:
		return Tenant
	case TLib:
		return Tenant
	}
	return UNKNOWN
}

func primaryCode(code Code) Code {
	switch code {
	case TQ:
		return Q
	case TLib:
		return QLib
	}
	return UNKNOWN
}

func composedCode(primary ID, embed ID) Code {
	if embed.Code() != Tenant {
		return UNKNOWN
	}
	switch primary.Code() {
	case Q:
		return TQ
	case QLib:
		return TLib
	}
	return UNKNOWN
}

// Composed is an ID that embeds another ID.
type Composed struct {
	full     ID
	primary  ID
	embedded ID
	strCache string
}

// ID returns the full ID (with the embedded ID)
func (c Composed) ID() ID {
	return c.full
}

// Primary returns the primary ID.
func (c Composed) Primary() ID {
	return c.primary
}

// Embedded returns the embedded ID.
func (c Composed) Embedded() ID {
	return c.embedded
}

func (c Composed) String() string {
	return c.strCache
}

func (c Composed) IsValid() bool {
	return c.full.IsValid() && c.embedded.IsValid()
}

// MarshalText implements custom marshaling using the string representation.
func (c Composed) MarshalText() ([]byte, error) {
	return []byte(c.String()), nil
}

// UnmarshalText implements custom unmarshaling from the string representation.
func (c *Composed) UnmarshalText(text []byte) error {
	parsed, err := FromString(string(text))
	if err != nil {
		return errors.E("unmarshal ID", errors.K.Invalid, err)
	}
	content := Decompose(parsed)
	*c = content
	return nil
}

func (c Composed) Explain() (res string) {
	if !c.full.IsValid() {
		return c.full.String() + " invalid"
	}

	sb := strings.Builder{}

	explain := func(id ID) {
		sb.WriteString(id.String())
		sb.WriteString(" ")
		sb.WriteString(codeToName[id.Code()])
		sb.WriteString(" 0x")
		sb.WriteString(hex.EncodeToString(id.Bytes()))
	}

	explain(c.full)
	if c.Embedded().IsValid() {
		sb.WriteString("\n  primary : ")
		explain(c.Primary())
		sb.WriteString("\n  embedded: ")
		explain(c.Embedded())
	}

	return sb.String()
}
