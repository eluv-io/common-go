package utc

import (
	"encoding/json"
	"time"

	"github.com/qluvio/content-fabric/errors"
)

const (
	ISO8601             = "2006-01-02T15:04:05.000Z07:00"
	ISO8601DateOnlyNoTZ = "2006-01-02"
	ISO8601DateOnly     = "2006-01-02Z07:00"
	ISO8601NoMilli      = "2006-01-02T15:04:05Z07:00"
	ISO8601NoSec        = "2006-01-02T15:04Z07:00"
	ISO8601NoMilliNoTZ  = "2006-01-02T15:04:05"
	ISO8601NoSecNoTZ    = "2006-01-02T15:04"

	yearZeroOffsetSec = int64(62167219200) // the year 0 returns a unix time of -62167219200
)

var (
	Zero    = UTC{}                                                           // Zero is the zero value of UTC
	Min     = Zero                                                            // 0000-01-01T00:00:00.000000000 = Zero
	Max     = New(time.Date(9999, 12, 31, 23, 59, 59, 999_999_999, time.UTC)) // 9999-12-31T23:59:59.999999999
	formats = []string{
		ISO8601,
		ISO8601DateOnlyNoTZ,
		ISO8601DateOnly,
		ISO8601NoMilli,
		ISO8601NoSec,
		ISO8601NoMilliNoTZ,
		ISO8601NoSecNoTZ,
	}
)

// New creates a new UTC instance from the given time. Use utc.Now() to get the
// current time.
func New(t time.Time) UTC {
	return UTC{Time: t.UTC(), mono: t}
}

// Now returns the current time as UTC instance. Now is a func *variable*, so
// it can be mocked in tests. See MockNow() function.
var Now = now

// now is the default, non-mocked value of Now.
func now() UTC {
	return New(time.Now())
}

// MockNowFn allows to replace the Now func variable with a mock function and
// returns a function to restore the default Now() implementation.
//
// Usage:
//	defer MockNow(func() UTC { ... })()
func MockNowFn(fn func() UTC) (restore func()) {
	Now = fn
	return ResetNow
}

// MockNow allows to replace the Now func variable with a function that returns
// the given constant time and returns itself a function to restore the default
// Now() implementation.
//
// Usage:
//	defer MockNow(utc.MustParse("2020-01-01"))()
func MockNow(time UTC) (restore func()) {
	return MockNowFn(func() UTC {
		return time
	})
}

// ResetNow resets the Now func to the default implementation.
func ResetNow() {
	Now = now
}

// UTC is a standard time.Time in the UTC timezone with marshaling to and from ISO 8601 / RFC 3339 format with fixed
// milliseconds: 2006-01-02T15:04:05.000Z
//
// Years smaller than "0000" and larger than "9999" cannot be marshaled to bytes, text, or JSON, and generate an error
// if attempted.
//
// time.Time keeps track of a "wall clock" for "time telling" as well as a "monotonic clock" for "time measurements" -
// see documentation of the time package. The monotonic clock is automatically stripped from a Time instance that
// results from a time operation (e.g. Add, Truncate) as well as timezone changes, unmarshalling, etc.
//
// Since UTC changes the timezone from "local" to "UTC", this also strips the monotonic clock. However, we want to be
// able to use UTC also for reliable time measurements... Hence we retain the original Time instance and use that for
// time measurements.
//
// Prefer the Equals() method over comparison with Go's == operator - the latter also compares the monotonic clock and
// Location, which might lead to undesired results. If time measurements are not needed, the monotonic clock can be
// removed with StripMono(). Also see the documentation of time.Time for this problem.
//
// See https://en.wikipedia.org/wiki/ISO_8601
// See https://tools.ietf.org/html/rfc3339
type UTC struct {
	time.Time           // time in UTC
	mono      time.Time // original time instance retained for mono clock
}

// Mono returns the time.Time instance for time measurement operations. Note that the returned instance has an actual
// monotonic clock only if the original Time instance, from which this UTC was created, had a monotonic clock. This is
// the case if created through utc.Now() (unless mocked) or New(time.Now())
func (u *UTC) Mono() time.Time {
	return u.mono
}

// StripMono returns a new UTC instance stripped of the monotonic clock.
//
// This is sometimes needed when comparing UTC instances with Go's == operator, which when used on time.Time instances,
// also compares the mono clock. See doc of time package.
func (u UTC) StripMono() UTC {
	return New(u.Time.Truncate(0))
}

// String returns the time formatted ISO 8601 format.
func (u UTC) String() string {
	s := []byte("0000-00-00T00:00:00.000Z")
	year, month, day := u.Date()
	hour, min, sec := u.Clock()
	millis := u.Nanosecond() / 1000000

	if year > 9999 {
		year = 9999
	} else if year < 0 {
		year = 0
	}
	s[3] = byte('0' + year%10)
	year /= 10
	s[2] = byte('0' + year%10)
	year /= 10
	s[1] = byte('0' + year%10)
	year /= 10
	s[0] = byte('0' + year)

	s[6] = byte('0' + month%10)
	s[5] = byte('0' + month/10)

	s[9] = byte('0' + day%10)
	s[8] = byte('0' + day/10)

	s[12] = byte('0' + hour%10)
	s[11] = byte('0' + hour/10)

	s[15] = byte('0' + min%10)
	s[14] = byte('0' + min/10)

	s[18] = byte('0' + sec%10)
	s[17] = byte('0' + sec/10)

	s[22] = byte('0' + millis%10)
	millis /= 10
	s[21] = byte('0' + millis%10)
	millis /= 10
	s[20] = byte('0' + millis)

	return string(s)
}

// UnixMilli returns the unix time in millis (milliseconds since 1970-01-01T00:00:00.000Z.
func (u UTC) UnixMilli() int64 {
	return u.Unix()*1000 + time.Duration(u.Nanosecond()).Milliseconds()
}

func (u UTC) Add(d time.Duration) UTC {
	return New(u.mono.Add(d))
}

func (u UTC) Sub(other UTC) time.Duration {
	return u.mono.Sub(other.mono)
}

func (u UTC) Truncate(d time.Duration) UTC {
	return New(u.mono.Truncate(d))
}

func (u UTC) Round(d time.Duration) UTC {
	return New(u.mono.Round(d))
}

func (u UTC) After(other UTC) bool {
	return u.mono.After(other.mono)
}

func (u UTC) Before(other UTC) bool {
	return u.mono.Before(other.mono)
}

func (u UTC) Equal(other UTC) bool {
	return u.Time.Equal(other.Time)
}

// MarshalJSON implements the json.Marshaler interface.
// Contrary to time.Time, it always marshals milliseconds, even if they are all
// zeros (i.e. 2006-01-02T15:04:05.000Z instead of 2006-01-02T15:04:05Z)
func (u UTC) MarshalJSON() ([]byte, error) {
	if u.IsZero() {
		return []byte(`""`), nil
	}
	// see time.Time.MarshalJSON()
	if y := u.Year(); y < 0 || y >= 10000 {
		// RFC 3339 is clear that years are 4 digits exactly.
		// See golang.org/issue/4556#c15 for more discussion.
		return nil, errors.E("UTC.MarshalJSON", "reason", "year outside of range [0,9999]")
	}
	return []byte(`"` + u.String() + `"`), nil
}

func (u *UTC) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	return u.UnmarshalText([]byte(s))
}

func (u *UTC) UnmarshalText(data []byte) error {
	utc, err := FromString(string(data))
	if err != nil {
		return err
	}
	*(&u.Time) = utc.Time
	*(&u.mono) = u.Time
	return nil
}

// MarshalText implements the encoding.TextMarshaler interface.
// Contrary to time.Time, it always marshals milliseconds, even if they are all
// zeros (i.e. 2006-01-02T15:04:05.000Z instead of 2006-01-02T15:04:05Z)
func (u UTC) MarshalText() ([]byte, error) {
	if u.IsZero() {
		return []byte(`""`), nil
	}
	if y := u.Year(); y < 0 || y >= 10000 {
		// RFC 3339 is clear that years are 4 digits exactly.
		// See golang.org/issue/4556#c15 for more discussion.
		return nil, errors.E("UTC.MarshalText", "reason", "year outside of range [0,9999]")
	}
	return []byte(u.String()), nil
}

// MarshalBinary implements the encoding.BinaryMarshaler interface.
func (u UTC) MarshalBinary() ([]byte, error) {
	if u.IsZero() {
		return nil, nil
	}
	if y := u.Year(); y < 0 || y >= 10000 {
		// RFC 3339 is clear that years are 4 digits exactly.
		// See golang.org/issue/4556#c15 for more discussion.
		return nil, errors.E("UTC.MarshalJSON", "reason", "year outside of range [0,9999]")
	}

	// marshal/unmarshal adapted from time.Time
	// reduces binary form to 9 bytes (from 15) because of the limited year
	// range.

	// add the year zero offset in order to ensure that sec is 0 or positive
	sec := uint64(u.Unix() + yearZeroOffsetSec)
	nsec := uint32(u.Nanosecond())
	enc := []byte{
		//timeBinaryVersion, // byte 0 : version
		//byte(sec >> 56),   // bytes 1-8: seconds
		//byte(sec >> 48),
		//byte(sec >> 40),
		byte(sec >> 32),
		byte(sec >> 24),
		byte(sec >> 16),
		byte(sec >> 8),
		byte(sec),
		byte(nsec >> 24), // bytes 9-12: nanoseconds
		byte(nsec >> 16),
		byte(nsec >> 8),
		byte(nsec),
		//byte(offsetMin >> 8), // bytes 13-14: zone offset in minutes
		//byte(offsetMin),
	}
	return enc, nil
}

// UnmarshalBinary implements the encoding.BinaryUnmarshaler interface.
func (u *UTC) UnmarshalBinary(data []byte) error {
	buf := data
	if len(buf) == 0 {
		// the zero value
		*u = UTC{}
		return nil
	}

	expectedLen := /*sec*/ 5 + /*nsec*/ 4
	if len(buf) != expectedLen {
		return errors.E("UTC.UnmarshalBinary",
			"reason", "invalid length",
			"expected_length", expectedLen,
			"actual_length", len(buf))
	}

	sec := uint64(buf[4]) | uint64(buf[3])<<8 | uint64(buf[2])<<16 | uint64(buf[1])<<24 |
		uint64(buf[0])<<32

	buf = buf[5:]
	nsec := uint32(buf[3]) | uint32(buf[2])<<8 | uint32(buf[1])<<16 | uint32(buf[0])<<24

	*(&u.Time) = time.Unix(int64(sec)-yearZeroOffsetSec, int64(nsec)).UTC()
	*(&u.mono) = u.Time
	return nil
}

// FromString parses the given time string.
func FromString(s string) (UTC, error) {
	var t time.Time
	var err error
	if s == "" {
		return Zero, nil
	}
	for _, format := range formats {
		t, err = time.ParseInLocation(format, s, time.UTC)
		if err == nil {
			return New(t.UTC()), nil
		}
	}
	return Zero, errors.E("parse", err, "utc", s)
}

// MustParse parses the given time string according to ISO 8601 format,
// panicking in case of errors.
func MustParse(s string) UTC {
	utc, err := FromString(s)
	if err != nil {
		panic(err)
	}
	return utc
}

// Parse parses the given time value string with the provided layout - see
// Time.Parse()
func Parse(layout string, value string) (UTC, error) {
	t, err := time.Parse(layout, value)
	if err != nil {
		return Zero, err
	}
	return New(t), nil
}

// Unix returns the local Time corresponding to the given Unix time,
// sec seconds and nsec nanoseconds since January 1, 1970 UTC.
// It is valid to pass nsec outside the range [0, 999999999].
// Not all sec values have a corresponding time value. One such
// value is 1<<63-1 (the largest int64 value).
func Unix(sec int64, nsec int64) UTC {
	return New(time.Unix(sec, nsec))
}

// UnixMilli returns the local Time corresponding to the given Unix time in
// milliseconds since January 1, 1970 UTC. This is the reverse operation of
// UTC.UnixMilli()
func UnixMilli(millis int64) UTC {
	return New(time.Unix(millis/1000, int64(time.Millisecond)*(millis%1000)))
}

func Since(t UTC) time.Duration {
	return time.Since(t.mono)
}

func Until(t UTC) time.Duration {
	return time.Until(t.mono)
}
