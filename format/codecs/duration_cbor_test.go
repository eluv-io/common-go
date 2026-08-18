package codecs

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/codecutil"
)

var cborCodec = NewCborV2Codec()

// TestDurationSpecCBORBackwardCompat verifies that a plain, untagged CBOR integer of nanoseconds - the wire format
// duration.Spec has always used, and what's already persisted in existing metadata from before any CBOR tag for it
// existed - still decodes correctly into a *duration.Spec, regardless of what tag 45 is currently bound to (see
// durationSpecRevert). This encodes through the same MultiCodec (header included), just as a plain int64, with no tag
// or marshaler involved at all; it must keep decoding as nanoseconds, not be misread as some other unit.
func TestDurationSpecCBORBackwardCompat(t *testing.T) {
	buf := &bytes.Buffer{}
	require.NoError(t, cborCodec.Encoder(buf).Encode(int64(200*time.Millisecond))) // 200,000,000 ns, no tag/marshaler

	var s duration.Spec
	err := cborCodec.Decoder(bytes.NewReader(buf.Bytes())).Decode(&s)
	require.NoError(t, err)
	require.Equal(t, 200*time.Millisecond, s.Duration())
}

// TestDurationSpecCBORRoundTrip verifies a duration.Spec round-trips through the codec both via a typed decode and,
// deliberately, inside a generic interface{} decode - the shape used when reading raw metadata (e.g. SQMDR.Get()).
// Since bb0f8437 was reverted, a generic decode of tag 45 no longer produces a duration.Spec directly - it comes back
// as the plain durationSpecRevert int64 the tag is now bound to, matching the bare-number wire format API clients
// expect (see durationSpecRevert's doc). The final MapDecode step confirms that value can still be recovered as a
// proper duration.Spec on demand, for callers that need one.
func TestDurationSpecCBORRoundTrip(t *testing.T) {
	spec := duration.MustParse("200ms")
	var revert = durationSpecRevert(spec)

	buf := &bytes.Buffer{}
	require.NoError(t, cborCodec.Encoder(buf).Encode(&revert))

	var decoded duration.Spec
	require.NoError(t, cborCodec.Decoder(bytes.NewReader(buf.Bytes())).Decode(&decoded))
	require.Equal(t, spec, decoded)

	var generic interface{}
	require.NoError(t, cborCodec.Decoder(bytes.NewReader(buf.Bytes())).Decode(&generic))
	genSpec, ok := generic.(durationSpecRevert)
	require.True(t, ok, "expected durationSpecRevert, got %T: %v", generic, generic)
	require.EqualValues(t, spec, genSpec)

	var ds duration.Spec
	err := codecutil.MapDecode(generic, &ds)
	require.NoError(t, err)
	require.Equal(t, spec, ds)
}

// TestDurationSpecCBORWrapped verifies the pointer-field shape actually used in production configs (e.g.
// SrtConnectionConfig.Latency *duration.Spec) round-trips correctly.
func TestDurationSpecCBORWrapped(t *testing.T) {
	type wrapper struct {
		Latency *duration.Spec
	}

	spec := duration.MustParse("200ms")
	w := wrapper{Latency: &spec}

	buf := &bytes.Buffer{}
	require.NoError(t, cborCodec.Encoder(buf).Encode(w))

	var decoded wrapper
	require.NoError(t, cborCodec.Decoder(bytes.NewReader(buf.Bytes())).Decode(&decoded))
	require.NotNil(t, decoded.Latency)
	require.Equal(t, 200*time.Millisecond, decoded.Latency.Duration())
}

// TestDurationSpecCBORWrapped verifies the pointer-field shape actually used in production configs (e.g.
// SrtConnectionConfig.Latency *duration.Spec) round-trips correctly.
func TestDurationCBORWrapped(t *testing.T) {
	type wrapper struct {
		Latency *duration.Duration
	}

	dur := duration.MustParseDuration("200ms")
	w := wrapper{Latency: &dur}

	buf := &bytes.Buffer{}
	require.NoError(t, cborCodec.Encoder(buf).Encode(w))

	var decoded wrapper
	require.NoError(t, cborCodec.Decoder(bytes.NewReader(buf.Bytes())).Decode(&decoded))
	require.NotNil(t, decoded.Latency)
	require.Equal(t, 200*time.Millisecond, decoded.Latency.Duration())
}

// TestDurationCBORSelfDescribing verifies duration.Spec's own MarshalCBOR/UnmarshalCBOR: encoding a plain Duration
// value produces a CBOR text string - String()'s output - rather than a bare integer, so a generic interface{} decode
// yields a plain, unambiguous Go string instead of a number that could be misread as the wrong unit.
func TestDurationCBORSelfDescribing(t *testing.T) {
	spec := duration.MustParseDuration("200ms")

	buf := &bytes.Buffer{}
	require.NoError(t, cborCodec.Encoder(buf).Encode(spec))

	var decoded duration.Duration
	require.NoError(t, cborCodec.Decoder(bytes.NewReader(buf.Bytes())).Decode(&decoded))
	require.Equal(t, spec, decoded)

	var generic interface{}
	require.NoError(t, cborCodec.Decoder(bytes.NewReader(buf.Bytes())).Decode(&generic))
	require.Equal(t, "200ms", generic, "a generic decode must yield a plain, unambiguous string")
}
