package codecs

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/utc-go"
)

func TestCompBasic(t *testing.T) {
	tc("test string").run(t)
	tc([]byte("test string")).run(t)

	tc(int8(99)).disableExactMatch().run(t)
	tc(int16(1699)).disableExactMatch().run(t)
	tc(int32(3299)).disableExactMatch().run(t)
	tc(int64(6499)).disableExactMatch().run(t)

	tc(uint8(77)).disableExactMatch().run(t)
	tc(uint16(1677)).disableExactMatch().run(t)
	tc(uint32(3277)).disableExactMatch().run(t)
	tc(uint64(6477)).run(t) // positive ints are decoded to uint64

	tc(int8(-99)).disableExactMatch().run(t)
	tc(int16(-1699)).disableExactMatch().run(t)
	tc(int32(-3299)).disableExactMatch().run(t)
	tc(int64(-6499)).run(t) // negative ints are decoded to int64

	tc(float32(5.74)).disableExactMatch().run(t)
	//goland:noinspection GoRedundantConversion
	tc(float64(-2135.987324)).run(t)

	tc(true).run(t)
	tc(false).run(t)

	tc([]int8{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}).withCmp(cmpSlice[int8]).run(t)
	tc([]int{10, 11, 12, 13, 14, 15, 16, 17, 18, 19}).withCmp(cmpSlice[int]).run(t)
	tc([]float32{0.1, 1.1, 2.1, 3.1, 4.1, 5.1, 6.1, 7.1, 8.1, 9.1}).withCmp(cmpSlice[float32]).run(t)

	// V1: FA --> 26 	IEEE 754 Single-Precision Float (32 bits follow)
	// 8A             # array(10)
	//   FA C0800000 # primitive(3229614080)
	//   FA C0400000 # primitive(3225419776)
	//   FA C0000000 # primitive(3221225472)
	//   FA BF800000 # primitive(3212836864)
	//   FA 00000000 # primitive(0)
	//   FA 3F800000 # primitive(1065353216)
	//   FA 40000000 # primitive(1073741824)
	//   FA 40400000 # primitive(1077936128)
	//   FA 40800000 # primitive(1082130432)
	//   FA 40A00000 # primitive(1084227584)
	//
	// V2: F9 --> 25 	IEEE 754 Half-Precision Float (16 bits follow)
	// 8A         # array(10)
	//   F9 C400 # primitive(50176)
	//   F9 C200 # primitive(49664)
	//   F9 C000 # primitive(49152)
	//   F9 BC00 # primitive(48128)
	//   F9 0000 # primitive(0)
	//   F9 3C00 # primitive(15360)
	//   F9 4000 # primitive(16384)
	//   F9 4200 # primitive(16896)
	//   F9 4400 # primitive(17408)
	//   F9 4500 # primitive(17664)
	tc([]float32{-4, -3, -2, -1, 0, 1, 2, 3, 4, 5}).disableBinaryCheck().withCmp(cmpSlice[float32]).run(t)

	tc(map[string]any{"a": 1, "b": "two", "c": true, "e": 238974.234}).withCmp(func(t *testing.T, val map[string]any, v1, v2 any) {
		for k, v := range val {
			valV1 := v1.(map[string]any)[k]
			valV2 := v2.(map[string]any)[k]
			assert.EqualValues(t, v, valV1)
			assert.EqualValues(t, v, valV2)
			assert.EqualValues(t, valV1, valV2)
		}
	}).run(t)
}

func TestCompTime(t *testing.T) {
	cmpTime := func(t *testing.T, val time.Time, v1, v2 any) {
		assert.Equal(t, val.UTC(), v1)                                           // v1 codec decodes to UTC
		assert.Equal(t, val.Truncate(0), v2.(time.Time).Round(time.Microsecond)) // v2 codec decodes to local and does not strip nanos
		assert.Equal(t, v1, v2.(time.Time).UTC().Round(time.Microsecond))
	}
	mustParse := func(s string) time.Time {
		ts, err := time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", s)
		require.NoError(t, err)
		return ts
	}

	tc(time.Now()).withCmp(cmpTime).disableBinaryCheck().run(t)

	// this produces slightly different cbor encodings:
	// v1: c1fb41d98e84687b8f08 -> 1(1715081633.930605)
	//                          -> C1                     # tag(1)
	//                               FB 41D98E84687B8F08 # primitive(4744980381751283464)
	// v2: c1fb41d98e84687b8f09 -> 1(1715081633.9306052)
	//                          -> C1                     # tag(1)
	//                               FB 41D98E84687B8F09 # primitive(4744980381751283465)
	tc(mustParse("2024-05-07 13:33:53.930605 +0200 CEST")).withCmp(cmpTime).disableBinaryCheck().run(t)

	// this time produces exacty same cbor encoding
	tc(mustParse("2024-05-07 14:00:09.730238 +0200 CEST")).withCmp(cmpTime).run(t)
}

func TestCompUTC(t *testing.T) {
	cmpUtc := func(t *testing.T, val utc.UTC, v1 any, v2 any) {
		assert.Equal(t, val.StripMono(), v1)
		assert.Equal(t, val.StripMono(), v2)
		assert.Equal(t, v1, v2)
	}
	tc(utc.Now()).withCmp(cmpUtc).run(t)
	tc(utc.MustParse("2024-05-07T11:33:53.930605")).withCmp(cmpUtc).run(t)
	tc(utc.MustParse("2024-05-07T12:00:09.730238")).withCmp(cmpUtc).run(t)
}

// testCase defines a test case for validating encoding and decoding equivalence between the CborV1Codec and
// CborV2Codec. The "value" is encoded and decoded with both codecs and the result verified against the original and
// against each other.
type testCase[T any] struct {
	// the source value to encode/decode
	value T
	// an optional verification function
	cmp func(t *testing.T, val T, v1, v2 any)
	// if enabled, ensure equality of CBOR encoding
	binaryCheck bool
	// if enabled, uses assert.Equal for comparisons, otherwise assert.EqualValues
	exactMatch bool
}

func (c *testCase[T]) withCmp(cmp func(t *testing.T, val T, v1, v2 any)) *testCase[T] {
	c.cmp = cmp
	return c
}

func (c *testCase[T]) run(t *testing.T) {
	t.Run(fmt.Sprint(c.value), func(t *testing.T) {
		t.Run("interface{}", func(t *testing.T) {
			var v1, v2 any
			encV1 := c.encdec(t, CborV1Codec, c.value, &v1)
			encV2 := c.encdec(t, CborV2Codec, c.value, &v2)
			if c.binaryCheck {
				assert.Equal(t, encV1, encV2, "v1=%x\nv2=%x\nsource=%s\n", encV1, encV2, c.value)
			}
			c.compare(t, c.exactMatch, v1, v2)
		})
		t.Run("zero-value", func(t *testing.T) {
			var v1, v2 T // zero values
			encV1 := c.encdec(t, CborV1Codec, c.value, &v1)
			encV2 := c.encdec(t, CborV2Codec, c.value, &v2)
			if c.binaryCheck {
				assert.Equal(t, encV1, encV2, "v1=%x\nv2=%x\nsource=%s\n", encV1, encV2, c.value)
			}
			c.compare(t, true, v1, v2)
		})
	})
}

func (c *testCase[T]) compare(t *testing.T, exact bool, v1, v2 any) {
	if c.cmp == nil {
		if exact {
			assert.Equal(t, c.value, v1)
			assert.Equal(t, c.value, v2)
			assert.Equal(t, v1, v2)
		} else {
			assert.EqualValues(t, c.value, v1)
			assert.EqualValues(t, c.value, v2)
			assert.EqualValues(t, v1, v2)
		}
		return
	}

	c.cmp(t, c.value, v1, v2)
}

func (c *testCase[T]) encdec(t *testing.T, codec Codec, val any, decodeTo any) []byte {
	encoded := c.encode(t, codec, val)
	c.decode(t, codec, encoded, decodeTo)
	return encoded
}

func (c *testCase[T]) encode(t *testing.T, codec Codec, val any) (encoded []byte) {
	buf := &bytes.Buffer{}
	err := codec.Encoder(buf).Encode(val)
	require.NoError(t, err)
	return buf.Bytes()
}

func (c *testCase[T]) decode(t *testing.T, codec Codec, bts []byte, val any) {
	reader := bytes.NewReader(bts)
	err := codec.Decoder(reader).Decode(val)
	require.NoError(t, err)
}

func (c *testCase[T]) disableBinaryCheck() *testCase[T] {
	c.binaryCheck = false
	return c
}

// disableExactMatch disables exact matching when decoding value into interface{}. Exact matching is still used for
// decoding into the zero value of the respective type.
func (c *testCase[T]) disableExactMatch() *testCase[T] {
	c.exactMatch = false
	return c
}

func tc[T any](value T) *testCase[T] {
	return &testCase[T]{
		value:       value,
		binaryCheck: true,
		exactMatch:  true,
	}
}

func cmpSlice[T any](t *testing.T, val []T, v1, v2 any) {
	if _, ok := v1.([]interface{}); ok { // decoded into interface{} ==> slice is []interface{}
		for idx, v := range val {
			require.EqualValues(t, v, v1.([]any)[idx])
			require.EqualValues(t, v, v2.([]any)[idx])
			require.Equal(t, v1, v2)
		}
	} else { // decoded into zero value ==> slice is []T
		require.Equal(t, val, v1)
		require.Equal(t, val, v2)
		require.Equal(t, v1, v2)
	}
}
