package duration_test

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
)

const (
	dns = duration.Duration(duration.Nanosecond)
	dus = duration.Duration(duration.Microsecond)
	dms = duration.Duration(duration.Millisecond)
	ds  = duration.Duration(duration.Second)
	dm  = duration.Duration(duration.Minute)
	dh  = duration.Duration(duration.Hour)
)

func TestDurationFormatting(t *testing.T) {
	assert.Equal(t, "1ns", dns.String())
	assert.Equal(t, "1µs", dus.String())
	assert.Equal(t, "1ms", dms.String())

	assert.Equal(t, "1.001µs", (dus + dns).String())
	assert.Equal(t, "1.000001ms", (dms + dns).String())
	assert.Equal(t, "1.000000001s", (ds + dns).String())
	assert.Equal(t, "1.001001ms", (dms + dus + dns).String())
	assert.Equal(t, "1.001001001s", (ds + dms + dus + dns).String())

	assert.Equal(t, "1s", ds.String())
	assert.Equal(t, "1m", dm.String())
	assert.Equal(t, "1h", dh.String())

	assert.Equal(t, "1m1s", (dm + ds).String())
	assert.Equal(t, "1h1s", (dh + ds).String())
	assert.Equal(t, "1h1m1s", (dh + dm + ds).String())

	assert.Equal(t, "1m0.000000001s", (dm + dns).String())
	assert.Equal(t, "1h0.000000001s", (dh + dns).String())
	assert.Equal(t, "1h1m1s", (dh + dm + ds).String())

	assert.Equal(t, "1h1m1.001001001s", (dh + dm + ds + dms + dus + dns).String())

	assert.Equal(t, "5µs", (5 * dus).String())
	assert.Equal(t, "10ns", (10 * dns).String())
	assert.Equal(t, "20ms", (20 * dms).String())
	assert.Equal(t, "200ms", (200 * dms).String())
	assert.Equal(t, "200ms", dur("200ms").String())
}

func TestDurationParsing(t *testing.T) {
	assert.Equal(t, dns, dur("1ns"))
	assert.Equal(t, dus, dur("1µs"))
	assert.Equal(t, dms, dur("1ms"))
	assert.Equal(t, 20*dms, dur("20ms"))
	assert.Equal(t, 20*time.Millisecond, dur("20ms").Duration())

	assert.Equal(t, dus+dns, dur("1.001µs"))
	assert.Equal(t, dms+dns, dur("1.000001ms"))
	assert.Equal(t, ds+dns, dur("1.000000001s"))
	assert.Equal(t, dms+dus+dns, dur("1.001001ms"))
	assert.Equal(t, ds+dms+dus+dns, dur("1.001001001s"))

	assert.Equal(t, ds, dur("1s"))
	assert.Equal(t, dm, dur("1m"))
	assert.Equal(t, dh, dur("1h"))

	assert.Equal(t, dm+ds, dur("1m1s"))
	assert.Equal(t, dh+ds, dur("1h1s"))
	assert.Equal(t, dh+dm+ds, dur("1h1m1s"))

	assert.Equal(t, dm+dns, dur("1m0.000000001s"))
	assert.Equal(t, dh+dns, dur("1h0.000000001s"))
	assert.Equal(t, dh+dm+ds, dur("1h1m1s"))

	assert.Equal(t, dh+dm+ds+dms+dus+dns, dur("1h1m1.001001001s"))

	// bare numbers (no unit) are seconds - the same convention for an integer-looking value as for a float,
	// regardless of magnitude.
	assert.Equal(t, ds, dur("1"))
	assert.Equal(t, 1_000_000_000*ds, dur("1000000000"))

	// floats
	assert.Equal(t, 100*dms, dur("0.1"))
	assert.Equal(t, ds+222*dms+333*dus+444*dns, dur("1.222333444"))
}

func TestParseDuration(t *testing.T) {
	assert.Equal(t, dm, duration.ParseDuration("invalid", "1m"))
	assert.Equal(t, dm, duration.ParseDuration("1m", "2m"))
	assert.Panics(t, func() { duration.ParseDuration("invalid", "invalid") })
}

func TestDurationJSON(t *testing.T) {
	s := "1h1m1.001001001s"
	d := dur(s)

	b, err := json.Marshal(d)
	assert.NoError(t, err)
	assert.Equal(t, "\""+s+"\"", string(b))

	var unmarshalled duration.Duration
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, d, unmarshalled)
}

func TestDurationUnmarshalJSON(t *testing.T) {
	tests := []struct {
		text      string
		wrapper   bool
		want      duration.Duration
		wantError bool
	}{
		{
			text: `"15ms"`,
			want: 15 * dms,
		},
		{
			text: `"99.5s"`,
			want: 99*ds + 500*dms,
		},
		{
			text: `"99.5"`, // numeric string (no unit)
			want: 99*ds + 500*dms,
		},
		{
			text: `99.5`, // number
			want: 99*ds + 500*dms,
		},
		{
			text: `"99"`, // integer string (no unit)
			want: 99 * ds,
		},
		{
			text: `99`, // integer number
			want: 99 * ds,
		},
		{
			text:    `{"dur": "15ms"}`,
			wrapper: true,
			want:    15 * dms,
		},
		{
			text:    `{"dur": "99.5s"}`,
			wrapper: true,
			want:    99*ds + 500*dms,
		},
		{
			text:    `{"dur": "99.5"}`, // numeric string (no unit)
			wrapper: true,
			want:    99*ds + 500*dms,
		},
		{
			text:    `{"dur": 99.5}`, // number
			wrapper: true,
			want:    99*ds + 500*dms,
		},
		{
			text:    `{"dur": "99"}`, // integer string (no unit)
			wrapper: true,
			want:    99 * ds,
		},
		{
			text:    `{"dur": 99}`, // integer number
			wrapper: true,
			want:    99 * ds,
		},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			var err error
			var dur duration.Duration
			if tt.wrapper {
				w := DurationWrapper{}
				err = json.Unmarshal([]byte(tt.text), &w)
				dur = w.Dur
			} else {
				err = json.Unmarshal([]byte(tt.text), &dur)
			}
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, dur, tt.want)
			}
		})
	}
}

type DurationWrapper struct {
	Dur duration.Duration `json:"dur,omitempty"`
}

func TestDurationWrappedJSON(t *testing.T) {
	str := "1h1m1.001001001s"
	d := dur(str)

	wrapper := DurationWrapper{
		Dur: d,
	}
	b, err := json.Marshal(wrapper)
	assert.NoError(t, err)
	assert.Contains(t, string(b), str)

	fmt.Println(string(b))

	var unmarshalled DurationWrapper
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, wrapper, unmarshalled)
}

// TestCBOR verifies duration.Duration's MarshalCBOR/UnmarshalCBOR: the wire format is a plain CBOR text string
// (String()'s output), so a generic (interface{}) decode yields that same string, never a bare number.
func TestDurationCBOR(t *testing.T) {
	str := "1h1m1.001001001s"
	d := dur(str)

	b, err := cbor.Marshal(d)
	assert.NoError(t, err)

	var decodedStr string
	assert.NoError(t, cbor.Unmarshal(b, &decodedStr))
	assert.Equal(t, str, decodedStr, "wire format must be a plain CBOR text string")

	var unmarshalled duration.Duration
	assert.NoError(t, cbor.Unmarshal(b, &unmarshalled))
	assert.Equal(t, d, unmarshalled)

	var generic interface{}
	assert.NoError(t, cbor.Unmarshal(b, &generic))
	assert.Equal(t, str, generic, "a generic decode must yield a plain, unambiguous string")
}

// TestUnmarshalCBOR covers UnmarshalCBOR's accepted wire shapes: a text string (current format, via FromString)
// and a bare integer (Duration's original nanosecond format, kept for backward compatibility) - both signs, since CBOR
// decodes a positive bare integer as uint64 and a negative one as int64, exercising both switch cases in
// UnmarshalCBOR - plus its error cases.
func TestDurationUnmarshalCBOR(t *testing.T) {
	tests := []struct {
		name      string
		data      func() []byte
		want      duration.Duration
		wantError bool
	}{
		{
			name: "text string with unit",
			data: func() []byte { b, _ := cbor.Marshal("15ms"); return b },
			want: 15 * dms,
		},
		{
			name: "text string, bare number (seconds)",
			data: func() []byte { b, _ := cbor.Marshal("99"); return b },
			want: 99 * ds,
		},
		{
			name: "legacy bare positive integer (nanoseconds, decodes generically as uint64)",
			data: func() []byte { b, _ := cbor.Marshal(int64(200 * time.Millisecond)); return b },
			want: 200 * dms,
		},
		{
			name: "legacy bare negative integer (nanoseconds, decodes generically as int64)",
			data: func() []byte { b, _ := cbor.Marshal(int64(-200 * time.Millisecond)); return b },
			want: -200 * dms,
		},
		{
			name:      "invalid string",
			data:      func() []byte { b, _ := cbor.Marshal("not a duration"); return b },
			wantError: true,
		},
		{
			name:      "unsupported CBOR type (float)",
			data:      func() []byte { b, _ := cbor.Marshal(1.5); return b },
			wantError: true,
		},
		{
			name:      "unsupported CBOR type (bool)",
			data:      func() []byte { b, _ := cbor.Marshal(true); return b },
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dur duration.Duration
			err := cbor.Unmarshal(tt.data(), &dur)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, dur)
			}
		})
	}
}

// TestWrappedCBOR verifies a struct field of type duration.Duration round-trips correctly through CBOR.
func TestDurationWrappedCBOR(t *testing.T) {
	str := "1h1m1.001001001s"
	dur := dur(str)

	wrapper := DurationWrapper{Dur: dur}
	b, err := cbor.Marshal(wrapper)
	assert.NoError(t, err)

	var unmarshalled DurationWrapper
	assert.NoError(t, cbor.Unmarshal(b, &unmarshalled))
	assert.Equal(t, wrapper, unmarshalled)
}

func dur(s string) duration.Duration {
	d, err := duration.DurationFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestDurationRound(t *testing.T) {
	tests := []struct {
		dur  string
		want string
	}{
		{"1ns", "1ns"},
		{"1µs", "1µs"},
		{"1ms", "1ms"},
		{"1s", "1s"},
		{"1m", "1m"},
		{"1h", "1h"},
		{"1.000444ms", "1ms"},
		{"1.000555ms", "1.001ms"},
		{"1.000444s", "1s"},
		{"1.000555s", "1.001s"},
		{"1m10s444ms", "1m10s"},
		{"1m10s555ms", "1m11s"},
		{"1h1m10s444ms", "1h1m10s"},
		{"1h1m10s555ms", "1h1m11s"},
	}
	for _, tt := range tests {
		t.Run(tt.dur+"->"+tt.want, func(t *testing.T) {
			dur := duration.MustParse(tt.dur)
			require.Equal(t, tt.want, dur.Round().String())
		})
	}
}

func TestDurationRoundTo(t *testing.T) {
	tests := []struct {
		dur      string
		want     string
		decimals int
	}{
		{"1ns", "1ns", 0},
		{"1ns", "1ns", 5},
		{"1ns", "1ns", -2},
		{"1µs", "1µs", 1},
		{"1ms", "1ms", 2},
		{"1s", "1s", 3},
		{"1m", "1m", 0},
		{"1h", "1h", 1},
		{"766.123µs", "766.12µs", 2},
		{"766.123µs", "766.1µs", 1},
		{"766.123µs", "766µs", 0},
		{"766.962µs", "766.96µs", 2},
		{"766.962µs", "767µs", 1},
		{"766.962µs", "767µs", 0},
		{"1.123444ms", "1.123ms", 3},
		{"1.123444ms", "1.12ms", 2},
		{"1.123444ms", "1.1ms", 1},
		{"1.123444ms", "1ms", 0},
		{"1.123555ms", "1.124ms", 3},
		{"1.123555ms", "1.12ms", 2},
		{"1.123555ms", "1.1ms", 1},
		{"1.123555ms", "1ms", 0},
		{"1.123444s", "1.123s", 3},
		{"1.123444s", "1.12s", 2},
		{"1.123444s", "1.1s", 1},
		{"1.123444s", "1s", 0},
		{"1.123555s", "1.124s", 3},
		{"1.123555s", "1.12s", 2},
		{"1.123555s", "1.1s", 1},
		{"1.123555s", "1s", 0},
		{"1m10s444ms", "1m10s", 2},
		{"1m10s444ms", "1m10s", 1},
		{"1m10s444ms", "1m10s", 0},
		{"1m10s555ms", "1m11s", 2},
		{"1m10s555ms", "1m11s", 1},
		{"1m10s555ms", "1m11s", 0},
		{"1h1m10s444ms", "1h1m10s", 2},
		{"1h1m10s444ms", "1h1m10s", 1},
		{"1h1m10s444ms", "1h1m10s", 0},
	}
	for _, tt := range tests {
		t.Run(tt.dur+","+strconv.Itoa(tt.decimals)+"->"+tt.want, func(t *testing.T) {
			dur := duration.MustParse(tt.dur)
			require.Equal(t, tt.want, dur.RoundTo(tt.decimals).String())
		})
	}
}

func TestDurationJsonCborRoundtrip(t *testing.T) {
	d := dur("1h1m10s444ms")

	btsCbor, err := cbor.Marshal(d)
	require.NoError(t, err)

	var anyCbor any
	err = cbor.Unmarshal(btsCbor, &anyCbor)
	require.NoError(t, err)

	btsJson, err := json.Marshal(anyCbor)
	require.NoError(t, err)

	var anyJson any
	err = json.Unmarshal(btsJson, &anyJson)
	require.NoError(t, err)

	// this is the difference compared to duration.Spec: the roundtrip matches exactly (strings)
	require.Equal(t, anyJson, anyCbor)
	require.IsType(t, anyJson, "a string")

	// and the full roundtrip back to Duration works!
	var d2 duration.Duration
	err = json.Unmarshal(btsJson, &d2)
	require.NoError(t, err)

	fmt.Printf("org:    %v\nparsed: %v\n", d, d2)
	require.Equal(t, d, d2)
}
