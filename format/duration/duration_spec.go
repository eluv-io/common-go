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

// Round rounds the duration to a value that produces a sensitive and human
// readable form that removes insignificant information with theses rules:
//	* nanos  are capped if d > 1 millisecond: 1.123444ms -> 1.123ms
//	* micros are capped if d > 1 second:      1.123555s  -> 1.124s
//	* millis are capped if d > 1 minute:      1m10s444ms -> 1m10s
func (s Spec) Round() Spec {
	return s.RoundTo(3)
}

// RoundTo rounds the duration to a "reasonable" value like Round, but also
// allows to choose the number of decimals [0-3] that are retained:
//	* 766.123µs, 2 decimals: 766.12µs
//	* 1.123444ms, 1 decimal:   1.1ms
//	* 1.123444s, 0 decimals:   1s
func (s Spec) RoundTo(decimals int) Spec {
	if decimals > 3 {
		decimals = 3
	}
	if decimals < 0 {
		decimals = 0
	}

	var to time.Duration
	d := time.Duration(s)
	switch {
	case d > time.Minute:
		return Spec(d.Round(time.Second))
	case d > time.Second:
		to = time.Millisecond
	case d > time.Millisecond:
		to = time.Microsecond
	case d > time.Microsecond:
		to = time.Nanosecond
	default:
		return s
	}

	factor := time.Duration(1)
	for i := 0; i < 3-decimals; i++ {
		factor *= 10
	}

	return Spec(d.Round(to * factor))
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
