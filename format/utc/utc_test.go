package utc_test

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/utc"
)

var oneBillion = time.Unix(1000000000, 0)

// result of fmt.Println(oneBillion.UTC().Format(utc.ISO8601Format))
const oneBillionString = "2001-09-09T01:46:40.000Z"

var rnd = rand.New(rand.NewSource(time.Now().UnixNano()))

var dates = func() []utc.UTC {
	d := []utc.UTC{
		utc.Zero,
		utc.Min,
		utc.Max,
		utc.New(oneBillion),
		utc.MustParse("1970-01-01T00:00:00.000Z"),
		utc.MustParse("2020-01-01T00:00:00.000Z"),
		utc.MustParse("2020-01-01T09:46:23.889Z"),
		utc.New(time.Now()),
		utc.Now(),
	}
	// add random dates
	for i := 0; i < 5; i++ {
		d = append(d, utc.UnixMilli(utc.Min.UnixMilli()+rnd.Int63n(utc.Max.UnixMilli())))
	}
	return d
}()

var yearTooSmall = utc.Min.Add(-time.Hour)
var yearTooLarge = utc.Max.Add(time.Hour)
var invalidISO8601 = []utc.UTC{yearTooSmall, yearTooLarge}

var durations = func() []time.Duration {
	d := []time.Duration{time.Millisecond, 5 * time.Second, 10 * time.Hour}
	// add random durations
	for i := 0; i < 5; i++ {
		max := 1_000_000 * int64(time.Hour)
		d = append(d, time.Duration(rnd.Int63n(2*max)-max))
	}
	return d
}()

func TestZero(t *testing.T) {
	assert.Equal(t, "0001-01-01T00:00:00.000Z", utc.Zero.String())
	assert.Equal(t, 1, utc.Zero.Year())
	assert.Equal(t, time.Month(1), utc.Zero.Month())
	assert.Equal(t, 1, utc.Zero.Day())
	assert.Equal(t, 0, utc.Zero.Hour())
	assert.Equal(t, 0, utc.Zero.Minute())
	assert.Equal(t, 0, utc.Zero.Second())
	assert.Equal(t, 0, utc.Zero.Nanosecond())

	zeroValue := utc.UTC{}
	require.True(t, zeroValue.IsZero())
	assertTimezone(t, zeroValue)
}

func TestMin(t *testing.T) {
	assert.Equal(t, "0000-01-01T00:00:00.000Z", utc.Min.String())
	assert.Equal(t, 0, utc.Min.Year())
	assert.Equal(t, time.Month(1), utc.Min.Month())
	assert.Equal(t, 1, utc.Min.Day())
	assert.Equal(t, 0, utc.Min.Hour())
	assert.Equal(t, 0, utc.Min.Minute())
	assert.Equal(t, 0, utc.Min.Second())
	assert.Equal(t, 0, utc.Min.Nanosecond())
}

func TestMax(t *testing.T) {
	assert.Equal(t, "9999-12-31T23:59:59.999Z", utc.Max.String())
	assert.Equal(t, 9999, utc.Max.Year())
	assert.Equal(t, time.Month(12), utc.Max.Month())
	assert.Equal(t, 31, utc.Max.Day())
	assert.Equal(t, 23, utc.Max.Hour())
	assert.Equal(t, 59, utc.Max.Minute())
	assert.Equal(t, 59, utc.Max.Second())
	assert.Equal(t, 999_999_999, utc.Max.Nanosecond())
}

func TestFormatting(t *testing.T) {
	fmt.Println(oneBillion.UTC().Format(utc.ISO8601))
	ut := utc.New(oneBillion)
	assert.Equal(t, oneBillionString, ut.String())
	assertTimezone(t, ut)
}

func TestFromString(t *testing.T) {
	tests := []struct {
		s       string
		want    time.Time
		wantErr bool
	}{
		{oneBillionString, oneBillion, false},
		{"2001-09-09Z", oneBillion.Truncate(24 * time.Hour), false},
		{"2001-09-09", oneBillion.Truncate(24 * time.Hour), false},
		{"2001-09-09T01:46:40Z", oneBillion.Truncate(time.Second), false},
		{"2001-09-09T02:46:40+01:00", oneBillion.Truncate(time.Second), false},
		{"2001-09-09T01:46:40", oneBillion.Truncate(time.Second), false},
		{"2001-09-09T01:46Z", oneBillion.Truncate(time.Minute), false},
		{"2001-09-09T01:46", oneBillion.Truncate(time.Minute), false},
		{"2001-09-09", oneBillion.Truncate(24 * time.Hour), false},
		{"2001-09-09-01:00", oneBillion.Truncate(24 * time.Hour).Add(time.Hour), false},
		{"2001-09-09 01:46", time.Time{}, true},
		{"0001-01-01T00:00:00.000000000", utc.Zero.Time, false},
		{"0000-01-01T00:00:00.000000000", utc.Min.Time, false},
		{"9999-12-31T23:59:59.999999999", utc.Max.Time, false},
	}

	for _, test := range tests {
		ut, err := utc.FromString(test.s)
		if test.wantErr {
			require.Error(t, err, test.s)
		} else {
			require.NoError(t, err)
			require.True(t, utc.New(test.want).Equal(ut), "%v | expected %v, actual %v", test.s, utc.New(test.want), ut)
			require.True(t, test.want.Equal(ut.Time), "%v | expected %v, actual %v", test.s, utc.New(test.want), ut)
			assertTimezone(t, ut)
		}
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		format  string
		s       string
		want    time.Time
		wantErr bool
	}{
		{"2006-01-02Z07:00", "2001-09-09Z", oneBillion.Add(-time.Hour - 46*time.Minute - 40*time.Second), false},
		{"2006-01-02Z07:00", "2001-09-09-08:00", oneBillion.Truncate(24 * time.Hour).Add(8 * time.Hour), false},
		{"02.01.2006 15:04Z07:00", "18.09.2001 14:33+02:00", utc.MustParse("2001-09-18T12:33Z").Time, false},
		{"02.01.2006 15:04Z07:00", "2001-09-09-08:00", time.Time{}, true},
	}

	for _, test := range tests {
		ut, err := utc.Parse(test.format, test.s)
		if test.wantErr {
			assert.Error(t, err, test.s)
		} else {
			assert.NoError(t, err)
			assert.True(t, utc.New(test.want).Equal(ut), "%v | expected %v, actual %v", test.s, utc.New(test.want), ut)
			assert.True(t, test.want.Equal(ut.Time), "%v | expected %v, actual %v", test.s, utc.New(test.want), ut)
			assertTimezone(t, ut)
		}
	}
}

func TestMockNow(t *testing.T) {
	defer utc.ResetNow()

	restore := utc.MockNow(utc.Zero)
	assert.True(t, utc.Now().Equal(utc.Zero))

	d2020 := utc.MustParse("2020-01-01")
	restore = utc.MockNow(d2020)
	assert.True(t, utc.Now().Equal(d2020))

	restore()
	timeNow := time.Now()
	utcNow := utc.Now()
	assert.True(t, timeNow.Before(utcNow.UTC()) || timeNow.Equal(utcNow.UTC()), "time.Now=%s utc.Now=%s", timeNow, utcNow)
}

func TestMockNowFn(t *testing.T) {
	defer utc.ResetNow()

	now := utc.Zero
	restore := utc.MockNowFn(func() utc.UTC {
		return now
	})
	assert.True(t, utc.Now().Equal(utc.Zero))

	for i := 0; i < 10; i++ {
		now = now.Add(time.Second)
		assert.True(t, utc.Now().Equal(now))
	}

	restore()
	timeNow := time.Now()
	utcNow := utc.Now()
	assert.True(t, timeNow.Before(utcNow.UTC()) || timeNow.Equal(utcNow.UTC()), "time.Now=%s utc.Now=%s", now, utcNow)
}

func TestSince(t *testing.T) {
	now := time.Now()
	nowUTC := utc.Now()
	sinceUTC := utc.Since(nowUTC)
	since := time.Since(now)
	sinceUTC2 := utc.Since(nowUTC)

	assert.LessOrEqual(t, sinceUTC, since)
	assert.LessOrEqual(t, sinceUTC, sinceUTC2)

	// test mocking
	defer utc.MockNow(nowUTC.Add(time.Minute))()
	assert.Equal(t, time.Minute, utc.Since(nowUTC))
}

func TestUntil(t *testing.T) {
	then := time.Now().Add(time.Second)
	thenUTC := utc.Now().Add(time.Second)
	untilUTC := utc.Until(thenUTC)
	until := time.Until(then)
	untilUTC2 := utc.Until(thenUTC)

	assert.LessOrEqual(t, until, untilUTC)
	assert.LessOrEqual(t, untilUTC2, untilUTC)

	// test mocking
	defer utc.MockNow(thenUTC.Add(-time.Minute))()
	assert.Equal(t, time.Minute, utc.Until(thenUTC))
}

func TestMustParse_panic(t *testing.T) {
	fn := func() (err error) {
		defer func() {
			// recover from panic
			if e := recover(); e != nil {
				err = errors.E("reason", e)
			}
		}()
		utc.MustParse("invalid date")
		return nil
	}
	err := fn()
	assert.Error(t, err)
}

func TestJSON(t *testing.T) {
	tests := []struct {
		utc           utc.UTC
		want          string
		compareString bool
	}{
		{utc.New(oneBillion), `"` + oneBillionString + `"`, true},
		{utc.New(time.Time{}), `""`, true}, // ensure zero time is marshalled to ""
		{utc.Now().Truncate(time.Millisecond), "", false},
	}

	for _, test := range tests {
		marshalled, err := json.Marshal(test.utc)
		assert.NoError(t, err)
		if test.compareString {
			assert.Equal(t, test.want, string(marshalled))
		}

		var unmarshalled utc.UTC
		err = json.Unmarshal(marshalled, &unmarshalled)
		assert.NoError(t, err)
		assert.True(t, test.utc.Equal(unmarshalled))
		assertTimezone(t, unmarshalled)
	}
}

func TestJSONUnmarshal(t *testing.T) {
	ut := utc.New(oneBillion)
	assertTimezone(t, ut)

	b, err := json.Marshal(ut)
	assert.NoError(t, err)
	assert.Equal(t, "\""+oneBillionString+"\"", string(b))

	tests := []struct {
		s       string
		want    time.Time
		wantErr bool
	}{
		{oneBillionString, oneBillion, false},
		{"2001-09-09Z", oneBillion.Truncate(24 * time.Hour), false},
		{"2001-09-09T01:46:40Z", oneBillion.Truncate(time.Second), false},
		{"2001-09-09T01:46Z", oneBillion.Truncate(time.Minute), false},
		{"2001-09-09 01:46", time.Time{}, true},
		{"", time.Time{}, false},
	}

	for _, test := range tests {
		var unmarshalled utc.UTC
		err = json.Unmarshal([]byte(`"`+test.s+`"`), &unmarshalled)
		if test.wantErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.True(t, utc.New(test.want).Equal(unmarshalled))
			assertTimezone(t, unmarshalled)
		}
	}
}

type Wrapper struct {
	UTC utc.UTC
}

func TestWrappedJSON(t *testing.T) {
	ut := utc.New(oneBillion)

	wrapper := Wrapper{
		UTC: ut,
	}
	b, err := json.Marshal(wrapper)
	assert.NoError(t, err)
	assert.Contains(t, string(b), oneBillionString)

	fmt.Println(string(b))

	var unmarshalled Wrapper
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.True(t, wrapper.UTC.Equal(unmarshalled.UTC))
	assertTimezone(t, unmarshalled.UTC)
}

func TestMarshalText(t *testing.T) {
	ut := utc.New(oneBillion)
	b, err := ut.MarshalText()
	require.NoError(t, err)
	require.Equal(t, oneBillionString, string(b))
}

func TestMarshalBinary(t *testing.T) {
	vals := []utc.UTC{
		utc.UTC{},
		utc.Now(),
		utc.MustParse("0000-01-01T00:00:00.000Z"),
		utc.MustParse("9999-12-31T23:59:59.999Z"),
	}
	for _, val := range vals {
		b, err := val.MarshalBinary()
		require.NoError(t, err)
		res := utc.UTC{}
		err = res.UnmarshalBinary(b)
		require.NoError(t, err)
		require.True(t, val.Equal(res), val.String())
	}
}

func TestUTC_MarshalJSON(t *testing.T) {
	testFnOneDate(t, func(t *testing.T, date utc.UTC) {
		marshalled, err := json.Marshal(date)
		require.NoError(t, err)
		var unmarshalled utc.UTC
		err = json.Unmarshal(marshalled, &unmarshalled)
		require.NoError(t, err)
		date = date.Truncate(time.Millisecond)
		assert.True(t, date.Equal(unmarshalled), "date=%s unmarshalled=%s", date, unmarshalled)
	})
	for _, date := range invalidISO8601 {
		marshalled, err := json.Marshal(date)
		assert.Error(t, err)
		assert.Nil(t, marshalled)
	}
	for _, str := range []string{"", "blub", "02.01.2006 15:04Z07:00"} {
		var date utc.UTC
		err := date.UnmarshalJSON([]byte(str))
		assert.Error(t, err)
	}
}

func TestUTC_MarshalText(t *testing.T) {
	testFnOneDate(t, func(t *testing.T, date utc.UTC) {
		marshalled, err := date.MarshalText()
		require.NoError(t, err)
		var unmarshalled utc.UTC
		err = unmarshalled.UnmarshalText(marshalled)
		require.NoError(t, err)
		date = date.Truncate(time.Millisecond)
		assert.True(t, date.Equal(unmarshalled), "date=%s unmarshalled=%s", date, unmarshalled)
	})
	for _, date := range invalidISO8601 {
		marshalled, err := date.MarshalText()
		assert.Error(t, err)
		assert.Nil(t, marshalled)
	}
}

func TestUTC_MarshalBinary(t *testing.T) {
	testFnOneDate(t, func(t *testing.T, date utc.UTC) {
		marshalled, err := date.MarshalBinary()
		require.NoError(t, err)
		var unmarshalled utc.UTC
		err = unmarshalled.UnmarshalBinary(marshalled)
		require.NoError(t, err)
		assert.True(t, date.Equal(unmarshalled), "date=%s unmarshalled=%s", date, unmarshalled)

		if date.IsZero() {
			assert.Nil(t, marshalled)
		}
	})
	for _, date := range invalidISO8601 {
		marshalled, err := date.MarshalBinary()
		assert.Error(t, err)
		assert.Nil(t, marshalled)
	}
	for _, bts := range [][]byte{{0, 0, 0}, {0, 0, 0, 0, 0, 0, 0, 0, 0, 0}} {
		t.Run(fmt.Sprint(len(bts), "bytes"), func(t *testing.T) {
			var date utc.UTC
			err := date.UnmarshalBinary(bts)
			assert.Error(t, err)
		})
	}
}

func TestUTC_String(t *testing.T) {
	vals := []utc.UTC{
		utc.UTC{},
		utc.Now(),
		utc.MustParse("0000-01-01T00:00:00.000Z"),
		utc.MustParse("9999-12-31T23:59:59.999Z"),
	}
	for _, val := range vals {
		assert.Equal(t, val.Time.Format(utc.ISO8601), val.String())
		fmt.Println(val)
	}

	// large years are capped at 9999
	large := utc.New(time.Date(12999, 1, 1, 1, 1, 1, 1, time.UTC))
	assert.Equal(t, "9999-01-01T01:01:01.000Z", large.String())

	// negative years are set to 0000
	negative := utc.New(time.Date(-12999, 1, 1, 1, 1, 1, 1, time.UTC))
	assert.Equal(t, "0000-01-01T01:01:01.000Z", negative.String())
}

func TestUnixMilli(t *testing.T) {
	base := utc.MustParse("1970-01-01T00:00:00.000Z")
	ms999AsNanos := int64(time.Millisecond * 999)
	truncToMillis := func(i time.Duration) time.Duration {
		return i / time.Millisecond * time.Millisecond
	}
	tests := []struct {
		date utc.UTC
		exp  int64
	}{
		{base.Add(math.MaxInt64), time.Duration(math.MaxInt64).Milliseconds()},
		{base, 0},
		{base.Add(time.Millisecond), 1},
		{base.Add(-time.Millisecond), -1},
		{base.Add(time.Hour), time.Hour.Milliseconds()},
		{base.Add(-time.Hour), -time.Hour.Milliseconds()},
		{base.Add(1_000_000 * time.Hour), 1_000_000 * time.Hour.Milliseconds()},
		{base.Add(-1_000_000 * time.Hour), -1_000_000 * time.Hour.Milliseconds()},
		{base.Add(truncToMillis(math.MaxInt64)), time.Duration(math.MaxInt64).Milliseconds()},
		{base.Add(truncToMillis(math.MinInt64)), time.Duration(math.MinInt64).Milliseconds()},
		{utc.Unix(2e9, 0), 2e12},
		{utc.Unix(3e12, 0), 3e15},
		{utc.Unix(4e15, 0), 4e18},
		{utc.Unix(2e9, ms999AsNanos), 2e12 + 999},
		{utc.Unix(3e12, ms999AsNanos), 3e15 + 999},
		{utc.Unix(4e15, ms999AsNanos), 4e18 + 999},
		{utc.Unix(-2e9, 0), -2e12},
		{utc.Unix(-3e12, 0), -3e15},
		{utc.Unix(-4e15, 0), -4e18},
		{utc.Unix(-2e9, ms999AsNanos), -2e12 + 999},
		{utc.Unix(-3e12, ms999AsNanos), -3e15 + 999},
		{utc.Unix(-4e15, ms999AsNanos), -4e18 + 999},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%s_%d", test.date.String(), test.exp), func(t *testing.T) {
			assert.Equal(t, test.exp, test.date.UnixMilli())
			recovered := utc.UnixMilli(test.exp)
			// need to truncate the test date to millis (i.e. cut of micros and
			// nanos) since the UnitMilli does that, too...
			trunc := test.date.Truncate(time.Millisecond)
			assert.True(t, trunc.Equal(recovered), recovered)
			assert.True(t, trunc.Equal(recovered))
		})
	}
}

func TestUTC_Mono(t *testing.T) {
	tests := []struct {
		name     string
		utc      utc.UTC
		wantMono bool
	}{
		{name: "utc.Now()", utc: utc.Now(), wantMono: true},
		{name: "utc.New(time.Now())", utc: utc.New(time.Now()), wantMono: true},
		{name: "utc.MustParse(\"2021-09-09T07:24:42.638Z\")", utc: utc.MustParse("2021-09-09T07:24:42.638Z"), wantMono: false},
		{name: "u: utc.Now.Truncate(0)", utc: utc.Now().Truncate(0), wantMono: false},
		{name: "u: utc.Now.StripMono()", utc: utc.Now().StripMono(), wantMono: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// the only way to figure out if a Time instance has a mono clock is to transform it to a string and check that
			// there is a suffix with the mono clock: e.g. 2021-09-09 09:18:18.909178 +0200 CEST m=+0.003535001
			asString := test.utc.Mono().String()
			if test.wantMono {
				require.Regexp(t, "m=[+-]\\d+", asString)
			} else {
				require.NotRegexp(t, "m=[+-]\\d+", asString)
			}
		})
	}
}

func TestUTC_Add(t *testing.T) {
	testFnOneDate(t, func(t *testing.T, date utc.UTC) {
		for _, dur := range durations {
			t.Run(dur.String(), func(t *testing.T) {
				assert.Equal(t, date.Add(dur).UTC(), date.Add(dur).Time)
			})
		}
	})
}

func TestUTC_Sub(t *testing.T) {
	testFnTwoDates(t, func(t *testing.T, date1, date2 utc.UTC) {
		assert.Equal(t, date1.Mono().Sub(date2.Mono()), date1.Sub(date2))
	})
}

func TestUTC_Round(t *testing.T) {
	testFnOneDate(t, func(t *testing.T, date utc.UTC) {
		for _, dur := range []time.Duration{time.Millisecond, 5 * time.Second, 10 * time.Hour} {
			assert.Equal(t, date.Mono().Round(dur).UTC(), date.Round(dur).UTC())
		}
	})
}

func TestUTC_After(t *testing.T) {
	testFnTwoDates(t, func(t *testing.T, date1, date2 utc.UTC) {
		assert.Equal(t, date1.Mono().After(date2.Mono()), date1.After(date2))
	})
}

func TestUTC_Before(t *testing.T) {
	testFnTwoDates(t, func(t *testing.T, date1, date2 utc.UTC) {
		assert.Equal(t, date1.Mono().Before(date2.Mono()), date1.Before(date2))
	})
}

func testFnOneDate(t *testing.T, fn func(t *testing.T, date utc.UTC)) {
	for i := 0; i < len(dates); i++ {
		date := dates[i]
		t.Run(fmt.Sprint(date), func(t *testing.T) {
			fn(t, date)
		})
	}
}

func testFnTwoDates(t *testing.T, fn func(t *testing.T, date1, date2 utc.UTC)) {
	for i := 0; i+1 < len(dates); i++ {
		date1 := dates[i]
		date2 := dates[i+1]
		t.Run(fmt.Sprint(date1, date2), func(t *testing.T) {
			fn(t, date1, date2)
		})
	}
}

func assertTimezone(t *testing.T, val utc.UTC) {
	zone, offset := val.Zone()
	require.Equal(t, 0, offset)
	require.Equal(t, "UTC", zone)
}

//  go test -v -bench "Benchmark" -benchtime 5s -run "none" github.com/qluvio/content-fabric/format/utc
//	goos: darwin
//	goarch: amd64
//	pkg: github.com/qluvio/content-fabric/format/utc
//	BenchmarkString/time.Time.String-8         	20580853	       286 ns/op	      32 B/op	       1 allocs/op
//	BenchmarkString/utc.UTC.StringOpt-8        	70914042	        82.5 ns/op	      32 B/op	       1 allocs/op
//	PASS
//	ok  	github.com/qluvio/content-fabric/format/utc	12.143s
func BenchmarkString(b *testing.B) {
	now := utc.Now()
	benchmarks := []struct {
		name string
		fn   func()
	}{
		{"time.Time.String", func() { _ = now.Time.Format(utc.ISO8601) }},
		{"utc.UTC.StringOpt", func() { _ = now.String() }},
	}
	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				bm.fn()
			}
		})
	}
}
