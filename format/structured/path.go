package structured

import (
	"strings"
)

const defaultSeparator = "/"

var (
	rfc6901Decoder = strings.NewReplacer("~1", "/", "~0", "~")
	rfc6901Encoder = strings.NewReplacer("~", "~0", "/", "~1")
)

type Path []string

func (p Path) String() string {
	return p.Format(defaultSeparator)
}

func NewPath(segments ...string) Path {
	return Path{}.append(segments...)
}

// AsPath returns a new path initialized from path and the given segments
// and is equivalent to: Path(path).CopyAppend(segments...)
func AsPath(path []string, segments ...string) Path {
	return Path(path).CopyAppend(segments...)
}

// PathWith returns a new path initialized from s and the given segments and is
// equivalent to: NewPath(append([]string{s}, segments...)...)
func PathWith(s string, segments ...string) Path {
	return Path{s}.append(segments...)
}

// CopyAppend makes a copy of this path, appends the given segments to the copy
// and returns the copy.
func (p Path) CopyAppend(segments ...string) Path {
	c := make(Path, len(p)+len(segments))
	copy(c, p)
	copy(c[len(p):], segments)
	return c
}

// append appends the given segments to this path. Since it might have to
// reallocate the underlying array, the returned Path value must be used.
func (p Path) append(segments ...string) Path {
	return append(p, segments...)
}

// Format formats this path as a string, using the given optional path separator.
// If no separator is specified, the default separator '/' is used.
func (p Path) Format(separator ...string) string {
	return p.FormatEnc(rfc6901Encoder, separator...)
}

// Format formats this path as a string, using the given path encoder and
// optional path separator.
// If no separator is specified, the default separator '/' is used.
func (p Path) FormatEnc(encoder *strings.Replacer, separator ...string) string {
	sep := resolve(separator)
	if len(p) == 0 {
		return sep
	}

	encoded := make([]string, len(p))
	n := len(p) * len(sep)
	for idx, seg := range p {
		if encoder == nil {
			encoded[idx] = seg
		} else {
			encoded[idx] = encoder.Replace(seg)
		}
		n += len(encoded[idx])
	}

	b := make([]byte, n)
	bp := 0
	for _, seg := range encoded {
		bp += copy(b[bp:], sep)
		bp += copy(b[bp:], seg)
	}
	return string(b)
}

// IsEmpty returns true if the path is nil or empty, false otherwise
func (p Path) IsEmpty() bool {
	return len(p) == 0
}

// MarshalText implements custom marshaling using the string representation.
func (p Path) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// UnmarshalText implements custom unmarshaling from the string representation.
func (p *Path) UnmarshalText(text []byte) error {
	parsed := ParsePath(string(text))
	*p = parsed
	return nil
}

// Equals returns true if this path is identical to the given path, false
// otherwise.
func (p Path) Equals(other Path) bool {
	return len(p) == len(other) && p.Contains(other)
}

// StartsWith returns true if p starts with the given path, false otherwise.
// It's an alias to Contains().
func (p Path) StartsWith(other Path) bool {
	return p.Contains(other)
}

// Contains returns true if the given path is contained in p, false otherwise.
// In other words: p starts with the provided path.
func (p Path) Contains(other Path) bool {
	if len(p) < len(other) {
		return false
	}

	for idx, seg := range other {
		if p[idx] != seg {
			return false
		}
	}
	return true
}

// CommonRoot returns the common parent path of this path and the provided path.
func (p Path) CommonRoot(other Path) Path {
	if p == nil {
		return other
	}
	idx := 0
	for ; idx < len(p) && idx < len(other); idx++ {
		if p[idx] != other[idx] {
			break
		}
	}
	return p[:idx]
}

// Clones this path.
func (p Path) Clone() Path {
	// see https://github.com/golang/go/wiki/SliceTricks
	return append(p[:0:0], p...)
}

// ParsePath parses the given path string into a Path object, using the given
// optional path separator and the default path decoder (RFC6901).
// If no separator is specified, the default separator '/' is used.
func ParsePath(path string, separator ...string) Path {
	return ParsePathDec(rfc6901Decoder, path, separator...)
}

// ParsePath parses the given path string into a Path object, using the given
// optional path separator and path decoder (may be nil). If no separator is
// specified, the default separator '/' is used.
func ParsePathDec(decoder *strings.Replacer, path string, separator ...string) Path {
	if path == "" {
		return nil
	}
	sep := resolve(separator)
	p := strings.Split(path, sep)
	if p[0] == "" {
		p = p[1:]
	}
	if p[len(p)-1] == "" {
		p = p[:len(p)-1]
	}

	if decoder != nil {
		for idx, seg := range p {
			p[idx] = decoder.Replace(seg)
		}
	}
	return p
}

func resolve(separator []string) string {
	sep := defaultSeparator
	if len(separator) > 0 {
		sep = separator[0]
	}
	return sep
}

// EscapeSeparators escapes all separators in the given path string, using the
// given optional path separator and corresponding path encoder (RFC6901). If no
// separator is specified, the default separator '/' is used.
func EscapeSeparators(path string, separator ...string) string {
	sep := resolve(separator)
	if sep == defaultSeparator {
		return rfc6901Encoder.Replace(path)
	}
	return strings.NewReplacer("~", "~0", sep, "~1").Replace(path)
}
