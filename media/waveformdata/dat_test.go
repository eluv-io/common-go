package waveformdata

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// fixtureV2 is a hand-encoded version 2 file, laid out per the audiowaveform data format specification: 24-byte
// little-endian header (version 2, flags 0 = 16-bit, 48000 Hz, 2400 samples per pixel, 2 buckets, 2 channels)
// followed by 8 int16 values [bucket][channel][min,max].
var fixtureV2 = []byte{
	0x02, 0x00, 0x00, 0x00, // version
	0x00, 0x00, 0x00, 0x00, // flags
	0x80, 0xbb, 0x00, 0x00, // sample rate 48000
	0x60, 0x09, 0x00, 0x00, // samples per pixel 2400
	0x02, 0x00, 0x00, 0x00, // length
	0x02, 0x00, 0x00, 0x00, // channels
	0x00, 0x80, 0xff, 0x7f, // bucket 0 ch 0: -32768, 32767
	0xf6, 0xff, 0x0a, 0x00, // bucket 0 ch 1: -10, 10
	0x9c, 0xff, 0x64, 0x00, // bucket 1 ch 0: -100, 100
	0x00, 0x00, 0x00, 0x00, // bucket 1 ch 1: 0, 0
}

var fixtureV2Values = []int16{-32768, 32767, -10, 10, -100, 100, 0, 0}

// fixtureV1 is a version 1, 8-bit, mono file: 20-byte header (flags 1 = 8-bit, 8000 Hz, 256 samples per pixel,
// 3 buckets) followed by 6 int8 values.
var fixtureV1 = []byte{
	0x01, 0x00, 0x00, 0x00, // version
	0x01, 0x00, 0x00, 0x00, // flags: 8-bit
	0x40, 0x1f, 0x00, 0x00, // sample rate 8000
	0x00, 0x01, 0x00, 0x00, // samples per pixel 256
	0x03, 0x00, 0x00, 0x00, // length
	0x80, 0x7f, // -128, 127
	0xff, 0x01, // -1, 1
	0x00, 0x00, // 0, 0
}

var fixtureV1Values = []int16{-128, 127, -1, 1, 0, 0}

func TestDecodeV2(t *testing.T) {
	w, err := Decode(bytes.NewReader(fixtureV2))
	require.NoError(t, err)
	require.Equal(t, 2, w.Version)
	require.Equal(t, 2, w.Channels)
	require.Equal(t, 48000, w.SampleRate)
	require.Equal(t, 2400, w.SamplesPerPixel)
	require.Equal(t, Bits16, w.Bits)
	require.Equal(t, 2, w.Length())
	require.Equal(t, fixtureV2Values, w.Data)
	require.Equal(t, "100ms", w.Duration().String())
	require.Equal(t, "50ms", w.BucketDuration().String())
}

func TestDecodeV1(t *testing.T) {
	w, err := Decode(bytes.NewReader(fixtureV1))
	require.NoError(t, err)
	require.Equal(t, 1, w.Version)
	require.Equal(t, 1, w.Channels)
	require.Equal(t, 8000, w.SampleRate)
	require.Equal(t, 256, w.SamplesPerPixel)
	require.Equal(t, Bits8, w.Bits)
	require.Equal(t, 3, w.Length())
	require.Equal(t, fixtureV1Values, w.Data)
}

func TestEncodeRoundTrip(t *testing.T) {
	w, err := Decode(bytes.NewReader(fixtureV2))
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, Encode(&buf, w))
	require.Equal(t, fixtureV2, buf.Bytes())

	// A version 1 8-bit file re-encodes as version 2 with the same values.
	w1, err := Decode(bytes.NewReader(fixtureV1))
	require.NoError(t, err)
	buf.Reset()
	require.NoError(t, Encode(&buf, w1))
	require.Len(t, buf.Bytes(), headerSizeV2+6)
	back, err := Decode(&buf)
	require.NoError(t, err)
	require.Equal(t, 2, back.Version)
	require.Equal(t, Bits8, back.Bits)
	require.Equal(t, 1, back.Channels)
	require.Equal(t, fixtureV1Values, back.Data)
}

func TestDecodeErrors(t *testing.T) {
	_, err := Decode(bytes.NewReader(fixtureV2[:10]))
	require.Error(t, err)

	_, err = Decode(bytes.NewReader(fixtureV2[:headerSizeV2+3]))
	require.Error(t, err, "short data section")

	bad := append([]byte(nil), fixtureV2...)
	bad[0] = 3
	_, err = Decode(bytes.NewReader(bad))
	require.Error(t, err, "unsupported version")

	bad = append([]byte(nil), fixtureV2...)
	bad[8], bad[9] = 0, 0
	_, err = Decode(bytes.NewReader(bad))
	require.Error(t, err, "zero sample rate")
}

func TestJSONRoundTrip(t *testing.T) {
	w, err := Decode(bytes.NewReader(fixtureV2))
	require.NoError(t, err)

	js, err := json.Marshal(w)
	require.NoError(t, err)
	require.JSONEq(t, `{"version":2,"channels":2,"sample_rate":48000,"samples_per_pixel":2400,"bits":16,"length":2,`+
		`"data":[-32768,32767,-10,10,-100,100,0,0]}`, string(js))

	var back Waveform
	require.NoError(t, json.Unmarshal(js, &back))
	require.Equal(t, w, &back)

	// length must match the data
	err = json.Unmarshal([]byte(`{"version":2,"channels":2,"sample_rate":48000,"samples_per_pixel":2400,"bits":16,`+
		`"length":3,"data":[-32768,32767,-10,10,-100,100,0,0]}`), &back)
	require.Error(t, err)

	// an empty waveform marshals with an empty data array, not null
	js, err = json.Marshal(New(1, 48000, 2400))
	require.NoError(t, err)
	require.Contains(t, string(js), `"data":[]`)
}

func TestValidate(t *testing.T) {
	w := New(2, 48000, 2400)
	require.NoError(t, w.Validate())

	w.Data = []int16{1, 2, 3}
	require.Error(t, w.Validate(), "data not a multiple of channels*2")

	w = New(2, 48000, 2400)
	w.Bits = 12
	require.Error(t, w.Validate())

	w = New(0, 48000, 2400)
	require.Error(t, w.Validate())
	require.Equal(t, 0, w.Length())
}
