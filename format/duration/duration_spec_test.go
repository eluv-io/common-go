package duration_test

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/format/duration"
)

const (
	ns = duration.Nanosecond
	us = duration.Microsecond
	ms = duration.Millisecond
	s  = duration.Second
	m  = duration.Minute
	h  = duration.Hour
)

func TestFormatting(t *testing.T) {
	assert.Equal(t, "1ns", ns.String())
	assert.Equal(t, "1µs", us.String())
	assert.Equal(t, "1ms", ms.String())

	assert.Equal(t, "1.001µs", (us + ns).String())
	assert.Equal(t, "1.000001ms", (ms + ns).String())
	assert.Equal(t, "1.000000001s", (s + ns).String())
	assert.Equal(t, "1.001001ms", (ms + us + ns).String())
	assert.Equal(t, "1.001001001s", (s + ms + us + ns).String())

	assert.Equal(t, "1s", s.String())
	assert.Equal(t, "1m", m.String())
	assert.Equal(t, "1h", h.String())

	assert.Equal(t, "1m1s", (m + s).String())
	assert.Equal(t, "1h1s", (h + s).String())
	assert.Equal(t, "1h1m1s", (h + m + s).String())

	assert.Equal(t, "1m0.000000001s", (m + ns).String())
	assert.Equal(t, "1h0.000000001s", (h + ns).String())
	assert.Equal(t, "1h1m1s", (h + m + s).String())

	assert.Equal(t, "1h1m1.001001001s", (h + m + s + ms + us + ns).String())

	assert.Equal(t, "5µs", (5 * us).String())
	assert.Equal(t, "10ns", (10 * ns).String())
	assert.Equal(t, "20ms", (20 * ms).String())
	assert.Equal(t, "200ms", (200 * ms).String())
	assert.Equal(t, "200ms", from("200ms").String())
}

func TestParsing(t *testing.T) {
	assert.Equal(t, ns, from("1ns"))
	assert.Equal(t, us, from("1µs"))
	assert.Equal(t, ms, from("1ms"))
	assert.Equal(t, 20*ms, from("20ms"))
	assert.Equal(t, 20*time.Millisecond, from("20ms").Duration())

	assert.Equal(t, us+ns, from("1.001µs"))
	assert.Equal(t, ms+ns, from("1.000001ms"))
	assert.Equal(t, s+ns, from("1.000000001s"))
	assert.Equal(t, ms+us+ns, from("1.001001ms"))
	assert.Equal(t, s+ms+us+ns, from("1.001001001s"))

	assert.Equal(t, s, from("1s"))
	assert.Equal(t, m, from("1m"))
	assert.Equal(t, h, from("1h"))

	assert.Equal(t, m+s, from("1m1s"))
	assert.Equal(t, h+s, from("1h1s"))
	assert.Equal(t, h+m+s, from("1h1m1s"))

	assert.Equal(t, m+ns, from("1m0.000000001s"))
	assert.Equal(t, h+ns, from("1h0.000000001s"))
	assert.Equal(t, h+m+s, from("1h1m1s"))

	assert.Equal(t, h+m+s+ms+us+ns, from("1h1m1.001001001s"))

	// no units
	assert.Equal(t, s, from("1"))
	assert.Equal(t, 10*s, from("10"))
	assert.Equal(t, 100*ms, from("0.1"))
	assert.Equal(t, s+222*ms+333*us+444*ns, from("1.222333444"))
}

func TestJSON(t *testing.T) {
	s := "1h1m1.001001001s"
	d := from(s)

	b, err := json.Marshal(d)
	assert.NoError(t, err)
	assert.Equal(t, "\""+s+"\"", string(b))

	var unmarshalled duration.Spec
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, d, unmarshalled)
}

func TestUnmarshalJSON(t *testing.T) {
	tests := []struct {
		text      string
		wrapper   bool
		want      duration.Spec
		wantError bool
	}{
		{
			text: `"15ms"`,
			want: 15 * duration.Millisecond,
		},
		{
			text: `"99.5s"`,
			want: 99*duration.Second + 500*duration.Millisecond,
		},
		{
			text: `"99.5"`, // numeric string (no unit)
			want: 99*duration.Second + 500*duration.Millisecond,
		},
		{
			text: `99.5`, // number
			want: 99*duration.Second + 500*duration.Millisecond,
		},
		{
			text:    `{"spec": "15ms"}`,
			wrapper: true,
			want:    15 * duration.Millisecond,
		},
		{
			text:    `{"spec": "99.5s"}`,
			wrapper: true,
			want:    99*duration.Second + 500*duration.Millisecond,
		},
		{
			text:    `{"spec": "99.5"}`, // numeric string (no unit)
			wrapper: true,
			want:    99*duration.Second + 500*duration.Millisecond,
		},
		{
			text:    `{"spec": 99.5}`, // number
			wrapper: true,
			want:    99*duration.Second + 500*duration.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			var err error
			var spec duration.Spec
			if tt.wrapper {
				w := Wrapper{}
				err = json.Unmarshal([]byte(tt.text), &w)
				spec = w.Spec
			} else {
				err = json.Unmarshal([]byte(tt.text), &spec)
			}
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, spec, tt.want)
			}
		})
	}
}

type Wrapper struct {
	Spec duration.Spec `json:"spec,omitempty"`
}

func TestWrappedJSON(t *testing.T) {
	str := "1h1m1.001001001s"
	spec := from(str)

	wrapper := Wrapper{
		Spec: spec,
	}
	b, err := json.Marshal(wrapper)
	assert.NoError(t, err)
	assert.Contains(t, string(b), str)

	fmt.Println(string(b))

	var unmarshalled Wrapper
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, wrapper, unmarshalled)
}

func from(s string) duration.Spec {
	d, err := duration.FromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestRound(t *testing.T) {
	tests := []struct {
		spec string
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
		t.Run(tt.spec+"->"+tt.want, func(t *testing.T) {
			spec := duration.MustParse(tt.spec)
			require.Equal(t, tt.want, spec.Round().String())
		})
	}
}

func TestRoundTo(t *testing.T) {
	tests := []struct {
		spec     string
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
		t.Run(tt.spec+","+strconv.Itoa(tt.decimals)+"->"+tt.want, func(t *testing.T) {
			spec := duration.MustParse(tt.spec)
			require.Equal(t, tt.want, spec.RoundTo(tt.decimals).String())
		})
	}
}
