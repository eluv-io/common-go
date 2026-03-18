package duration

import (
	"strconv"
	"time"

	"github.com/eluv-io/errors-go"
)

// Millis is a duration that marshals as a float number of millis with 3 digits after the decimal point (e.g. "1.532")
type Millis time.Duration

func (s Millis) String() string {
	return strconv.FormatFloat(s.AsFloat(), 'f', 3, 64)
}

// MarshalText implements custom marshaling using the string representation.
func (s Millis) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalText implements custom unmarshaling from the string representation.
func (s *Millis) UnmarshalText(text []byte) error {
	parsed, err := MillisFromString(string(text))
	if err != nil {
		return errors.E("unmarshal duration", errors.K.Invalid, err)
	}
	*s = parsed
	return nil
}

func (s Millis) MarshalJSON() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalJSON implements custom unmarshaling. It supports unmarshalling from
//   - human readable strings with units: "1h15m"
//   - numeric strings without units, interpreted as millis: "10.5"
//   - numeric values, interpreted as millis: 10.5
func (s *Millis) UnmarshalJSON(b []byte) error {
	if len(b) >= 2 && b[0] == '"' {
		return s.UnmarshalText(b[1 : len(b)-1])
	}

	str := string(b)
	f, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return errors.E("unmarshal duration", errors.K.Invalid, err)
	}
	*s = Millis(f * float64(time.Millisecond))
	return nil
}

func (s Millis) Duration() time.Duration {
	return time.Duration(s)
}

// MillisFromString parses the given duration string into a duration spec.
func MillisFromString(s string) (Millis, error) {
	d, err := time.ParseDuration(s)
	if err == nil {
		return Millis(d), nil
	}

	f, err2 := strconv.ParseFloat(s, 64)
	if err2 == nil {
		return Millis(f * float64(time.Millisecond)), nil
	}

	return 0, errors.E("parse", err, "duration_spec", s)
}

func MillisFromFloat(f float64) Millis {
	return Millis(f * float64(time.Millisecond))
}

func (s Millis) AsFloat() float64 {
	msec := Spec(s) / Millisecond
	nsec := Spec(s) % Millisecond
	return float64(msec) + float64(nsec)/1e6
}
