package codecs_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
)

// TestDurationSpecCBORBackwardCompat verifies that a duration.Spec value stored before Spec was tag-registered -
// a bare, untagged CBOR integer of nanoseconds, since Spec's underlying type is int64 - still decodes correctly now
// that it is. This encodes through the same MultiCodec (header included), just as a plain int64 rather than a
// tagged Spec, matching what's already persisted in existing metadata; it must keep decoding as nanoseconds, not be
// misread as some other unit.
func TestDurationSpecCBORBackwardCompat(t *testing.T) {
	buf := &bytes.Buffer{}
	require.NoError(t, cborCodec.Encoder(buf).Encode(int64(200*time.Millisecond))) // 200,000,000 ns, no tag/marshaler

	var s duration.Spec
	err := cborCodec.Decoder(bytes.NewReader(buf.Bytes())).Decode(&s)
	require.NoError(t, err)
	require.Equal(t, 200*time.Millisecond, s.Duration())
}

// TestDurationSpecCBORRoundTrip verifies a duration.Spec round-trips through the codec, including inside a generic
// interface{} decode - the shape used when reading raw metadata (e.g. SQMDR.Get()) - to confirm the decoded value
// comes back as a duration.Spec, not a bare number. Spec has no custom CBOR marshaling of its own; this relies
// entirely on Spec's CBOR tag registration (codecs.go) identifying the type on the wire.
func TestDurationSpecCBORRoundTrip(t *testing.T) {
	spec := duration.MustParse("200ms")

	buf := &bytes.Buffer{}
	require.NoError(t, cborCodec.Encoder(buf).Encode(&spec))

	var decoded duration.Spec
	require.NoError(t, cborCodec.Decoder(bytes.NewReader(buf.Bytes())).Decode(&decoded))
	require.Equal(t, spec, decoded)

	var generic interface{}
	require.NoError(t, cborCodec.Decoder(bytes.NewReader(buf.Bytes())).Decode(&generic))
	genSpec, ok := generic.(duration.Spec)
	require.True(t, ok, "expected duration.Spec, got %T: %v", generic, generic)
	require.Equal(t, spec, genSpec)
}

// TestDurationSpecCBORWrapped verifies the pointer-field shape actually used in production configs
// (e.g. SrtConnectionConfig.Latency *duration.Spec) round-trips correctly.
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
