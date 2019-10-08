package link

import (
	"encoding/json"
	"fmt"
	"strings"

	"eluvio/errors"
	"eluvio/format/hash"
	"eluvio/format/structured"
	"eluvio/util/httputil"
)

const ABSOLUTE_LINK_PREFIX = "/qfab/"
const RELATIVE_LINK_PREFIX = "./"

type Selector string

var S = struct {
	None Selector
	Meta Selector
	File Selector
	Rep  Selector
	Blob Selector
}{
	None: "",
	Meta: "meta",
	File: "files",
	Rep:  "rep",
	Blob: "blob",
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
	err := link.Validate(true)
	if err != nil {
		return nil, err
	}
	return link, nil
}

// Link represents a reference to another structure in the content object data
// model. See /doc/design/content_data_model.md for details.
type Link struct {
	Target   *hash.Hash
	Selector Selector
	Path     structured.Path
	Off      int64
	Len      int64
	Props    map[string]interface{}
}

// String returns the Link as a string.
// Note: link properties are not encoded in the string!
//
// Examples:
//   "./meta/some/path"
//   "./files/some/path#40-49"
//   "/qfab/hqp_QmYtUc4iTCbbfVSDNKvtQqrfyezPPnFvE33wFmutw9PBBk"
//   "/qfab/hq__QmYtUc4iTCbbfVSDNKvtQqrfyezPPnFvE33wFmutw9PBBk/files/some/path"
//   "/qfab/hq__QmYtUc4iTCbbfVSDNKvtQqrfyezPPnFvE33wFmutw9PBBk/files/some/path#300-"
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

func (l Link) MarshalJSON() ([]byte, error) {
	m := make(map[string]interface{})
	m["/"] = l.String()
	for key, val := range l.Props {
		if key != "/" {
			m[key] = val
		}
	}
	return json.Marshal(m)
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
	linkText := structured.Wrap(m).Get("/").String()
	if linkText == "" {
		return errors.E("link.UnmarshalMap", errors.K.Invalid, "reason", "not a link", "map", m)
	}
	delete(m, "/")
	if len(m) > 0 {
		l.Props = m
	}
	err := l.UnmarshalText([]byte(linkText))
	if err == nil {
		err = l.Validate(true)
	}
	if err != nil {
		return err
	}
	return nil
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
		return e().Cause(err)
	}

	if !isRelative {
		l.Target, err = hash.FromString(p[0])
		if err != nil {
			return e().Cause(err)
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
		case S.Meta, S.File, S.Rep, S.Blob:
			// valid selector - continue
		default:
			return errors.E("unmarshal link", errors.K.Invalid, "reason", "unknown selector", "selector", p[0])
		}
	}
	if len(p) > 1 {
		l.Path = p[1:]
	}

	err = l.Validate(false)
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
		l.Off, l.Len, err = httputil.ParseByteRange(bRange)
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

func (l *Link) Validate(includeProps bool) error {
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
	case S.File, S.Rep:
		// no additional verification
	case S.Meta:
		if l.Off != 0 || l.Len != -1 {
			return e("reason", "byte range not allowed for meta link")
		}
	case S.Blob:
		if includeProps {
			if l.Props["data"] == nil {
				return e("reason", "no data specified for blob link")
			}
			_, err := l.AsBlob()
			if err != nil {
				return e(err)
			}
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

func (l *Link) AsBlob() (*BlobLink, error) {
	return NewBlobLink(l)
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
