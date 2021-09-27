package utc_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/format/codecs"
	"github.com/qluvio/content-fabric/format/utc"
)

var oneBillion = time.Unix(1000000000, 0)

// result of fmt.Println(oneBillion.UTC().Format(utc.ISO8601Format))
const oneBillionString = "2001-09-09T01:46:40.000Z"

func TestFormatting(t *testing.T) {
	fmt.Println(oneBillion.UTC().Format(utc.ISO8601))
	ut := utc.New(oneBillion)
	assert.Equal(t, oneBillionString, ut.String())
	assertTimezone(t, ut)
}

func TestParsing(t *testing.T) {
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
		{"0001-01-01T00:00:00.000000000", utc.Min.Time, false},
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

func TestCBOR(t *testing.T) {
	ut := utc.New(oneBillion)

	codec := codecs.NewCborCodec()
	buf := &bytes.Buffer{}

	err := codec.Encoder(buf).Encode(ut)
	require.NoError(t, err)

	var utDecoded utc.UTC
	err = codec.Decoder(buf).Decode(&utDecoded)
	require.NoError(t, err)

	require.Equal(t, ut.String(), utDecoded.String())
	assertTimezone(t, utDecoded)
}

func TestWrappedCBOR(t *testing.T) {
	ut := utc.New(oneBillion)

	wrapper := Wrapper{
		UTC: ut,
	}

	codec := codecs.NewCborCodec()
	buf := &bytes.Buffer{}

	err := codec.Encoder(buf).Encode(wrapper)
	require.NoError(t, err)

	var wrapperDecoded Wrapper
	err = codec.Decoder(buf).Decode(&wrapperDecoded)
	require.NoError(t, err)

	require.Equal(t, ut.String(), wrapperDecoded.UTC.String())
}

func TestGenericCBOR(t *testing.T) {
	ut := utc.New(oneBillion)
	m := map[string]interface{}{
		"billion": ut,
	}

	codec := codecs.NewCborCodec()
	buf := &bytes.Buffer{}

	err := codec.Encoder(buf).Encode(m)
	require.NoError(t, err)

	var mDecoded interface{}
	err = codec.Decoder(buf).Decode(&mDecoded)
	require.NoError(t, err)

	genDecoded := mDecoded.(map[string]interface{})["billion"]
	spew.Dump(genDecoded)
	utDecoded, ok := genDecoded.(utc.UTC)
	require.True(t, ok, genDecoded)
	require.Equal(t, ut.String(), utDecoded.String(), ok)
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

func TestZeroValue(t *testing.T) {
	zeroValue := utc.UTC{}
	require.True(t, zeroValue.IsZero())

	assertTimezone(t, zeroValue)
}

func TestString(t *testing.T) {
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

func TestMono(t *testing.T) {
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
