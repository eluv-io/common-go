package utc_test

import (
	"bytes"
	"encoding/json"
	"fmt"
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
		{"2001-09-09T01:46:40Z", oneBillion.Truncate(time.Second), false},
		{"2001-09-09T01:46Z", oneBillion.Truncate(time.Minute), false},
		{"2001-09-09", time.Time{}, true},
	}

	for _, test := range tests {
		ut, err := utc.FromString(test.s)
		if test.wantErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.True(t, test.want.Equal(ut.Time))
			assertTimezone(t, ut)
		}
	}
}

func TestJSON(t *testing.T) {
	ut := utc.New(oneBillion)
	assertTimezone(t, ut)

	b, err := json.Marshal(ut)
	assert.NoError(t, err)
	assert.Equal(t, "\""+oneBillionString+"\"", string(b))

	var unmarshalled utc.UTC
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, ut, unmarshalled)
	assertTimezone(t, unmarshalled)
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
	assert.Equal(t, wrapper, unmarshalled)
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
		require.Equal(t, val, res, val.String())
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

func assertTimezone(t *testing.T, zeroValue utc.UTC) {
	zone, offset := zeroValue.Zone()
	require.Equal(t, "UTC", zone)
	require.Equal(t, 0, offset)
}

// 	go test -v -bench "Benchmark" -benchtime 5s -run "none" github.com/qluvio/content-fabric/format/utc
//	goos: darwin
//	goarch: amd64
//	pkg: github.com/qluvio/content-fabric/format/utc
//	BenchmarkString/time.Time.String-8         	19764080	       289 ns/op
//	BenchmarkString/utc.UTC.StringOpt-8        	69861658	        83.4 ns/op
//	PASS
//	ok  	github.com/qluvio/content-fabric/format/utc	11.949s
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
			for i := 0; i < b.N; i++ {
				bm.fn()
			}
		})
	}
}
