package duration

import (
	"strconv"
	"strings"
	"time"

	"github.com/eluv-io/errors-go"
)

// Seconds is a duration that marshals as a float number of seconds (e.g. "1.5")
type Seconds time.Duration

func (s Seconds) String() string {
	// return fmt.Sprint(time.Duration(s).Seconds())
	res := strconv.FormatFloat(time.Duration(s).Seconds(), 'f', -1, 64)
	if !strings.Contains(res, ".") {
		res += ".0"
	}
	return res
}

// MarshalText implements custom marshaling using the string representation.
func (s Seconds) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalText implements custom unmarshaling from the string representation.
func (s *Seconds) UnmarshalText(text []byte) error {
	parsed, err := SecondsFromString(string(text))
	if err != nil {
		return errors.E("unmarshal duration", errors.K.Invalid, err)
	}
	*s = parsed
	return nil
}

func (s Seconds) MarshalJSON() ([]byte, error) {
	return []byte(s.String()), nil
}

// UnmarshalJSON implements custom unmarshaling. It supports unmarshalling from
//   - human readable strings with units: "1h15m"
//   - numeric strings without units, interpreted as seconds: "10.5"
//   - numeric values, interpreted as seconds: 10.5
func (s *Seconds) UnmarshalJSON(b []byte) error {
	if len(b) >= 2 && b[0] == '"' {
		return s.UnmarshalText(b[1 : len(b)-1])
	}

	str := string(b)
	f, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return errors.E("unmarshal duration", errors.K.Invalid, err)
	}
	*s = Seconds(f * float64(time.Second))
	return nil
}

func (s Seconds) Duration() time.Duration {
	return time.Duration(s)
}

// SecondsFromString parses the given duration string into a duration spec.
func SecondsFromString(s string) (Seconds, error) {
	d, err := time.ParseDuration(s)
	if err == nil {
		return Seconds(d), nil
	}

	f, err2 := strconv.ParseFloat(s, 64)
	if err2 == nil {
		return Seconds(f * float64(time.Second)), nil
	}

	return 0, errors.E("parse", err, "duration_spec", s)
}

func SecondsFromFloat(f float64) Seconds {
	return Seconds(f * float64(time.Second))
}
