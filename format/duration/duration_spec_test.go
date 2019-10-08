package duration_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"eluvio/format/duration"

	"github.com/stretchr/testify/assert"
)

const (
	ns = duration.Spec(time.Nanosecond)
	us = duration.Spec(time.Microsecond)
	ms = duration.Spec(time.Millisecond)
	s  = duration.Spec(time.Second)
	m  = duration.Spec(time.Minute)
	h  = duration.Spec(time.Hour)
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

type Wrapper struct {
	Spec duration.Spec
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
