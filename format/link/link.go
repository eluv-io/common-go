package link

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eluv-io/common-go/format/encryption"
	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/structured"
	"github.com/eluv-io/common-go/util/httputil/byterange"
	"github.com/eluv-io/errors-go"
)

const ABSOLUTE_LINK_PREFIX = "/qfab/"
const RELATIVE_LINK_PREFIX = "./"

type Selector string

var S = struct {
	None    Selector
	Meta    Selector
	File    Selector
	Rep     Selector
	Blob    Selector
	Bitcode Selector
}{
	None:    "",
	Meta:    "meta",
	File:    "files",
	Rep:     "rep",
	Blob:    "blob",
	Bitcode: "bc",
}

// NewLink creates a new Link. offAndLen is an optional variadic argument
// that specifies the optional offset and length corresponding to a byte range.
//
// Note: Use link.NewBuilder() for more flexibility in creating links.
func NewLink(target *hash.Hash, sel Selector, path structured.Path, offAndLen ...int64) (*Link, error) {
	var off int64 = 0
	var siz int64 = -1

	if len(offAndLen) > 1 {
		siz = offAndLen[1]
	}
	if len(offAndLen) > 0 {
		off = offAndLen[0]
	}

	link := &Link{
		Target:   target,
		Selector: sel,
		Path:     path,
		Off:      off,
		Len:      siz,
	}
	err := link.Validate()
	if err != nil {
		return nil, err
	}
	return link, nil
}

// Link represents a reference to another structure in the content object data
// model. See /doc/design/content_data_model.md for details.
type Link struct {
	// NOTE: DO NOT CHANGE FIELD TYPES, THEIR ORDER OR REMOVE ANY FIELDS SINCE STRUCT IS ENCODED AS ARRAY!
	_        struct{} `cbor:",toarray"` // encode struct as array
	Target   *hash.Hash
	Selector Selector
	Path     structured.Path
	Off      int64
	Len      int64
	Props    map[string]interface{}
	Extra    Extra
	Blob     *Blob
}

// String returns the Link as a string.
// Note: link properties are not encoded in the string!
//
// Examples:
//
//	"./meta/some/path"
//	"./files/some/path#40-49"
//	"/qfab/hqp_QmYtUc4iTCbbfVSDNKvtQqrfyezPPnFvE33wFmutw9PBBk"
//	"/qfab/hq__QmYtUc4iTCbbfVSDNKvtQqrfyezPPnFvE33wFmutw9PBBk/files/some/path"
//	"/qfab/hq__QmYtUc4iTCbbfVSDNKvtQqrfyezPPnFvE33wFmutw9PBBk/files/some/path#300-"
func (l Link) String() string {
	b := &strings.Builder{}
	addByteRange := func() {
		if l.Len != 0 && (l.Off != 0 || l.Len != -1) {
			if l.Len == -1 {
				b.WriteString(fmt.Sprintf("#%d-", l.Off))
			} else if l.Off == -1 {
				b.WriteString(fmt.Sprintf("#-%d", l.Len))
			} else {
				b.WriteString(fmt.Sprintf("#%d-%d", l.Off, l.Off+l.Len-1))
			}
		}
	}
	if !l.Target.IsNil() {
		b.WriteString(ABSOLUTE_LINK_PREFIX)
		b.WriteString(l.Target.String())
		switch l.Target.Type.Code {
		case hash.QPart:
			addByteRange()
			return b.String()
		}
	}
	if len(l.Selector) > 0 {
		if !l.Target.IsNil() {
			b.WriteString("/")
		} else {
			b.WriteString(RELATIVE_LINK_PREFIX)
		}
		b.WriteString(string(l.Selector))
		if len(l.Path) > 0 {
			b.WriteString(l.Path.String())
		}
	}
	addByteRange()
	return b.String()
}

func (l *Link) MarshalMap() map[string]interface{} {
	m := make(map[string]interface{})
	if l.Props != nil {
		_, _ = structured.Merge(m, nil, structured.Copy(l.Props))
	}
	m["/"] = l.String()
	if !l.Extra.IsEmpty() {
		m = structured.Wrap(structured.Merge(m, structured.Path{"."}, l.Extra.MarshalMap())).Map()
	}
	if l.Blob != nil {
		m = structured.Wrap(structured.Merge(m, nil, l.Blob.MarshalMap())).Map()
	}
	return m
}

func (l Link) MarshalJSON() ([]byte, error) {
	return json.Marshal(l.MarshalMap())
}

func (l *Link) UnmarshalJSON(data []byte) error {
	var m map[string]interface{}
	err := json.Unmarshal(data, &m)
	if err != nil {
		return err
	}
	return l.UnmarshalMap(m)
}

func (l *Link) UnmarshalMap(m map[string]interface{}) error {
	val := structured.Wrap(m)

	linkText := val.Get("/").String()
	if linkText == "" {
		return errors.E("link.UnmarshalMap", errors.K.Invalid, "reason", "not a link", "map", m)
	}
	val.Delete("/")

	if extra := val.Get("."); !extra.IsError() {
		err := l.Extra.UnmarshalValueAndRemove(extra)
		if err != nil {
			return err
		}
		if len(extra.Map()) == 0 {
			val.Delete(".")
		}
	}
	l.Extra.Container = "" // ignore container

	err := l.UnmarshalText([]byte(linkText))
	if err != nil {
		return err
	}

	if l.Selector == S.Blob {
		l.Blob = &Blob{
			EncryptionScheme: encryption.None,
		}
		err = l.Blob.UnmarshalValueAndRemove(val)
		if err != nil {
			return err
		}
	}

	l.Props = val.Map()
	if len(l.Props) == 0 {
		l.Props = nil
	}

	return l.Validate()
}

// MarshalCBORV1 converts this link to a generic map structure, suitable for encoding in CBOR.
//
// Deprecated: cbor un/marshaling is now performed with github.com/fxamacker/cbor/v2 and uses struct tags. Also, blob
// links are now not stored in link props anymore.
func (l *Link) MarshalCBORV1() map[string]interface{} {
	m := make(map[string]interface{})
	if !l.Target.IsNil() {
		m["Target"] = l.Target
	}
	if len(l.Selector) > 0 {
		m["Selector"] = l.Selector
	}
	if len(l.Path) > 0 {
		m["Path"] = l.Path
	}
	if l.Off > 0 {
		m["Off"] = l.Off
	}
	if l.Len > 0 {
		m["Len"] = l.Len
	}
	extra := l.Extra.MarshalMap()
	if len(extra) > 0 {
		m["Extra"] = extra
	}
	if l.Blob != nil {
		blobProps := l.Blob.MarshalMap()
		for k, v := range l.Props {
			blobProps[k] = v
		}
		l.Props = blobProps
	}
	l.cleanupProps()
	if len(l.Props) > 0 {
		m["Props"] = l.Props
	}
	return m
}

// UnmarshalCBORV1 unmarshals link data from a map - used with codecs.LinkConverter.
//
// Deprecated: cbor un/marshaling is now performed with github.com/fxamacker/cbor/v2 and uses struct tags. Also, blob
// links are now not stored in link props anymore.
func (l *Link) UnmarshalCBORV1(t map[string]interface{}) {
	var e interface{}
	var ok bool
	if e, ok = t["Target"]; ok {
		h := e.(hash.Hash)
		l.Target = &h
	}
	if e, ok = t["Selector"]; ok {
		l.Selector = Selector(e.(string))
	}
	if e, ok = t["Path"]; ok {
		l.Path = toPath(e.([]interface{}))
	}
	if e, ok = t["Off"]; ok {
		l.Off = toInt64(e)
	}
	if e, ok = t["Len"]; ok {
		l.Len = toInt64(e)
	} else {
		l.Len = -1
	}
	if e, ok = t["Extra"]; ok {
		l.Extra.UnmarshalMap(e.(map[string]interface{}))
	}
	if e, ok = t["Props"]; ok {
		l.Props = e.(map[string]interface{})
		// the new Link (version 2) expects blob-related properties to be removed and parsed into a Blob struct
		if l.Selector == S.Blob {
			l.Blob = &Blob{}
			_ = l.Blob.UnmarshalValueAndRemove(structured.Wrap(e))
		}
		if len(l.Props) == 0 {
			l.Props = nil
		}
	}
}

// MarshalText implements custom marshaling using the string representation.
func (l Link) MarshalText() ([]byte, error) {
	return []byte(l.String()), nil
}

// UnmarshalText implements custom unmarshaling from the string representation.
func (l *Link) UnmarshalText(text []byte) error {
	l.Selector = ""
	l.Off = 0
	l.Len = -1

	var err error
	e := errors.Template("unmarshal link", errors.K.Invalid, "link", string(text))
	s := string(text)

	var isRelative bool
	if strings.HasPrefix(s, RELATIVE_LINK_PREFIX) {
		isRelative = true
		s = s[len(RELATIVE_LINK_PREFIX):]
	} else if strings.HasPrefix(s, ABSOLUTE_LINK_PREFIX) {
		isRelative = false
		s = s[len(ABSOLUTE_LINK_PREFIX):]
	} else {
		return e("reason", fmt.Sprintf("invalid link - relative links must start with '%s', absolute links with '%s'", RELATIVE_LINK_PREFIX, ABSOLUTE_LINK_PREFIX))
	}

	p := structured.ParsePath(s)
	if len(p) == 0 {
		if isRelative {
			return e("reason", "selector required")
		} else {
			return e("reason", "content hash or content part hash required")
		}
	}

	if isRelative && Selector(p[0]) != S.Rep || !isRelative && (len(p) <= 1 || Selector(p[1]) != S.Rep) {
		p[len(p)-1], err = l.parseByteRange(p[len(p)-1])
	}

	if err != nil {
		return e(err)
	}

	if !isRelative {
		l.Target, err = hash.FromString(p[0])
		if err != nil {
			return e(err)
		}
		p = p[1:]
		switch l.Target.Type.Code {
		case hash.QPart:
			if len(p) > 0 {
				return e("reason", "content part links may not specify a selector or path")
			}
		}
	}

	if len(p) > 0 {
		l.Selector = Selector(p[0])
		switch l.Selector {
		case S.Meta, S.File, S.Rep, S.Blob, S.Bitcode:
			// valid selector - continue
		default:
			return errors.E("unmarshal link", errors.K.Invalid, "reason", "unknown selector", "selector", p[0])
		}
	}
	if len(p) > 1 {
		l.Path = p[1:]
	}

	err = l.validateCore()
	if err != nil {
		return errors.E("unmarshal link", err)
	}
	return nil
}

func (l *Link) parseByteRange(s string) (string, error) {
	var err error

	// check for a byte range at the end
	idx := strings.LastIndex(s, "#")
	if idx != -1 {
		bRange := s[idx+1:]
		l.Off, l.Len, err = byterange.Parse(bRange)
		if err != nil {
			// '#' is legal anywhere in the
			for _, r := range bRange {
				if !strings.ContainsRune("0123456789-", r) {
					// not a byte range at all, ignore the "range" and the error
					return s, nil
				}
			}
			return "", err
		}
		s = s[:idx]
	}
	return s, nil
}

func (l *Link) Validate() error {
	if err := l.validateCore(); err != nil {
		return err
	}

	e := errors.Template("validate link", errors.K.Invalid, "link", l)

	switch l.Selector {
	case S.Blob:
		if err := l.Blob.Validate(); err != nil {
			return e(err)
		}
	default:
		if l.Blob != nil {
			return e("reason", "blob struct must be nil")
		}
	}

	return nil
}

// validateCore validates only core data that is available when encoding/decoding to/from the string representation.
// This excludes Extra and Blob structs.
func (l *Link) validateCore() error {
	e := errors.Template("validate link", errors.K.Invalid, "link", l)
	if l.IsAbsolute() {
		switch l.Target.Type.Code {
		case hash.QPart:
			if !l.Path.IsEmpty() {
				return e("reason", "path not allowed for content part link")
			}
			if l.Selector != "" {
				return e("reason", "selector not allowed for content part link")
			}
			return nil
		}
	}

	if l.Selector == "" {
		return e("reason", "selector required")
	}

	switch l.Selector {
	case S.File, S.Rep, S.Blob, S.Bitcode:
		// no additional verification
	case S.Meta:
		if l.Off != 0 || l.Len != -1 {
			return e("reason", "byte range not allowed for meta link")
		}
	default:
		return e("reason", "invalid selector")
	}

	return nil
}

func (l *Link) IsAbsolute() bool {
	return !l.Target.IsNil()
}

func (l *Link) IsRelative() bool {
	return l.Target.IsNil()
}

func (l *Link) IsSigned() bool {
	return l.Extra.Authorization != ""
}

// Clone creates a "deepish" copy of this link: it duplicates the link struct
// and all nested values except the Target hash and Path.
func (l *Link) Clone() Link {
	res := *l
	res.Props = structured.Copy(l.Props).(map[string]interface{})
	res.Extra.AutoUpdate = res.Extra.AutoUpdate.Clone()
	if l.Blob != nil {
		blob := *l.Blob
		res.Blob = &blob
	}
	return res
}

func (l *Link) cleanupProps() {
	delete(l.Props, "/")
}

// FromString parses the given string and converts it to a Link.
// See Link.String()
func FromString(s string) (*Link, error) {
	var l Link
	err := l.UnmarshalText([]byte(s))
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// IsLink returns true if val is a Link or *Link
func IsLink(val interface{}) bool {
	switch val.(type) {
	case *Link:
		return true
	case Link:
		return true
	}
	return false
}

// AsLink returns the given value as *Link if it is a link.
// Otherwise returns nil.
func AsLink(val interface{}) *Link {
	switch l := val.(type) {
	case *Link:
		return l
	case Link:
		return &l
	}
	return nil
}

// ToLink tries to convert the given target object to a Link pointer. Returns the link pointer and true if successful,
// nil and false otherwise.
//
// The following objects are successfully converted:
//   - *Link (returned unchanged)
//   - Link
//   - a map[string]interface{} with a "/" key that unmarshals successfully to a link object
func ToLink(target interface{}) (*Link, bool) {
	switch t := target.(type) {
	case Link:
		return &t, true
	case *Link:
		return t, true
	case map[string]interface{}:
		lo, found := t["/"]
		if found {
			if _, ok := lo.(string); ok {
				l := &Link{}
				err := l.UnmarshalMap(t)
				if err == nil {
					return l, true
				}
			}
		}
	}
	return nil, false
}

// Converts the given value to an int64 if it is any of the possible Go integer
// types. Returns 0 otherwise.
func toInt64(v interface{}) int64 {
	switch t := v.(type) {
	// signed
	case int:
		return int64(t)
	case int64:
		return t
	case int32:
		return int64(t)
	case int16:
		return int64(t)
	case int8:
		return int64(t)
	// unsigned
	case uint:
		return int64(t)
	case uint64:
		return int64(t)
	case uint32:
		return int64(t)
	case uint16:
		return int64(t)
	case uint8:
		return int64(t)
	}
	return 0
}

func toPath(p []interface{}) structured.Path {
	s := make([]string, len(p))
	for idx, val := range p {
		s[idx] = val.(string)
	}
	return s
}
