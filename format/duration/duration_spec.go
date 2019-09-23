package duration

import (
	"strings"
	"time"

	"github.com/qluvio/content-fabric/errors"
)

// Spec represents a time duration. It provides marshaling to and from
// a human readable format, e.g. 1h15m or 200ms
type Spec time.Duration

// String returns the duration spec formatted like time.Duration.String(), but
// omits zero values.
// Examples:
//   1h0m0s is formatted as 1h
//   1h0m5s is formatted as 1h5s
func (s Spec) String() string {
	d := s.Duration()
	f := d.String()

	r := d / time.Second
	if d > time.Second {
		if r%60 == 0 {
			f = strings.Replace(f, "0s", "", 1)
		}
		if (r/60)%60 == 0 {
			f = strings.Replace(f, "0m", "", 1)
		}
	}
	return f
}

// MarshalText implements custom marshaling using the string representation.
func (s Spec) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalText implements custom unmarshaling from the string representation.
func (s *Spec) UnmarshalText(text []byte) error {
	parsed, err := FromString(string(text))
	if err != nil {
		return errors.E("unmarshal duration", errors.K.Invalid, err)
	}
	*s = parsed
	return nil
}

func (s Spec) Duration() time.Duration {
	return time.Duration(s)
}

// FromString parses the given duration string into a duration spec.
func FromString(s string) (Spec, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, errors.E("parse", err, "duration_spec", s)
	}
	return Spec(d), nil
}

// MustParse parses the given duration string into a duration spec, panicking in
// case of errors.
func MustParse(s string) Spec {
	spec, err := FromString(s)
	if err != nil {
		panic(err)
	}
	return spec
}

// Parse parses the given duration string into a duration spec, returning the
// parsed default in case of errors. Panics if the default cannot be parsed.
func Parse(s string, def string) Spec {
	spec, err := FromString(s)
	if err != nil {
		return MustParse(def)
	}
	return spec
}
