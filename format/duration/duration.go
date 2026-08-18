package duration

import (
	"strconv"
	"strings"
	"time"

	"github.com/fxamacker/cbor/v2"

	"github.com/eluv-io/errors-go"
)

// Duration represents a time duration. It provides marshaling to and from a human-readable format, e.g. 1h15m or 200ms.
//
// This is a replacement for duration.Spec, which is deprecated due to inconsistent JSON (string) and CBOR (int)
// marshaling.
type Duration time.Duration

// String returns the duration formatted like time.Duration.String(), but
// omits zero values.
// Examples:
//
//	1h0m0s is formatted as 1h
//	1h0m5s is formatted as 1h5s
func (s Duration) String() string {
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
func (s Duration) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalText implements custom unmarshaling from the string representation.
func (s *Duration) UnmarshalText(text []byte) error {
	parsed, err := DurationFromString(string(text))
	if err != nil {
		return errors.E("unmarshal duration", errors.K.Invalid, err)
	}
	*s = parsed
	return nil
}

// UnmarshalJSON implements custom unmarshaling. It supports unmarshalling from
//   - human readable strings with units: "1h15m"
//   - numeric strings without units, interpreted as seconds: "10.5"
//   - numeric values, interpreted as seconds: 10.5
func (s *Duration) UnmarshalJSON(b []byte) error {
	if len(b) >= 2 && b[0] == '"' {
		return s.UnmarshalText(b[1 : len(b)-1])
	}

	f, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return errors.E("unmarshal duration", errors.K.Invalid, err)
	}
	*s = Duration(f * float64(Second))
	return nil
}

// MarshalCBOR implements cbor.Marshaler, encoding as a CBOR text string (String()'s output) instead of a bare
// integer of the underlying nanosecond value - so a generic (interface{}) CBOR decode yields a plain, unambiguous
// Go string (e.g. "200ms"), mirroring MarshalText/JSON.
func (s Duration) MarshalCBOR() ([]byte, error) {
	return cbor.Marshal(s.String())
}

// UnmarshalCBOR implements cbor.Unmarshaler. Accepts a CBOR text string (the current wire format, via FromString)
// or a bare CBOR integer (Duration's original wire format - nanoseconds, matching its underlying time.Duration - kept
// for backward compatibility with data persisted before this method existed).
func (s *Duration) UnmarshalCBOR(data []byte) error {
	var v any
	if err := cbor.Unmarshal(data, &v); err != nil {
		return errors.E("unmarshal duration", errors.K.Invalid, err)
	}
	switch val := v.(type) {
	case string:
		parsed, err := DurationFromString(val)
		if err != nil {
			return errors.E("unmarshal duration", errors.K.Invalid, err)
		}
		*s = parsed
	case int64:
		*s = Duration(val)
	case uint64:
		*s = Duration(int64(val))
	default:
		return errors.E("unmarshal duration", errors.K.Invalid, "reason", "unsupported CBOR type", "type", val)
	}
	return nil
}

func (s Duration) Duration() time.Duration {
	return time.Duration(s)
}

// Round rounds the duration to a value that produces a sensible and human
// readable form that removes insignificant information with theses rules:
//   - nanos  are capped if d > 1 millisecond: 1.123444ms -> 1.123ms
//   - micros are capped if d > 1 second:      1.123555s  -> 1.124s
//   - millis are capped if d > 1 minute:      1m10s444ms -> 1m10s
func (s Duration) Round() Duration {
	return s.RoundTo(3)
}

// RoundTo rounds the duration to a "reasonable" value like Round, but also
// allows to choose the number of decimals [0-3] that are retained:
//   - 766.123µs, 2 decimals: 766.12µs
//   - 1.123444ms, 1 decimal:   1.1ms
//   - 1.123444s, 0 decimals:   1s
//
// For durations greater than one minute, decimals is ignored and the result is always rounded to the nearest second.
func (s Duration) RoundTo(decimals int) Duration {
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
		return Duration(d.Round(time.Second))
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

	return Duration(d.Round(to * factor))
}

// DurationFromString parses the given duration string into a Duration.
func DurationFromString(s string) (Duration, error) {
	d, err := time.ParseDuration(s)
	if err == nil {
		return Duration(d), nil
	}

	f, err2 := strconv.ParseFloat(s, 64)
	if err2 == nil {
		return Duration(f * float64(Second)), nil
	}

	return 0, errors.E("parse", err, "duration_Duration", s)
}

// MustParseDuration parses the given duration string into a Duration, panicking in case of errors.
func MustParseDuration(s string) Duration {
	dur, err := DurationFromString(s)
	if err != nil {
		panic(err)
	}
	return dur
}

// ParseDuration parses the given duration string into a Duration, returning the parsed default in case of errors.
// Panics if the default cannot be parsed.
func ParseDuration(s string, def string) Duration {
	dur, err := DurationFromString(s)
	if err != nil {
		return MustParseDuration(def)
	}
	return dur
}
