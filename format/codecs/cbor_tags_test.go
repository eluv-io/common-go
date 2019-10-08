package codecs_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/qluvio/content-fabric/format"
	"github.com/qluvio/content-fabric/format/codecs"
	"github.com/qluvio/content-fabric/format/hash"
	"github.com/qluvio/content-fabric/format/link"
	"github.com/qluvio/content-fabric/util/maputil"

	"github.com/stretchr/testify/require"
)

var codec = codecs.NewCborCodec()

func TestIDTag(t *testing.T) {
	factory := format.NewFactory()
	runTests(t, factory.GenerateQID())
}

func TestHashTag(t *testing.T) {
	val, err := hash.FromString("hq__2cwnvxLFFeFy6jNxzLeKBG43Xo4HKHab1TxV9k6JS3ByTseM1C1PjFizGAn6pWBkZhtcH")
	require.NoError(t, err)
	runTests(t, *val)
}

func TestLinkTag(t *testing.T) {
	tests := []struct {
		link *link.Link
	}{
		{link: newLink(t, "./meta/some/path")},
		{link: newLink(t, "./files/some/path#10-19")},
		{link: newLink(t, "/qfab/hq__2cwnvxLFFeFy6jNxzLeKBG43Xo4HKHab1TxV9k6JS3ByTseM1C1PjFizGAn6pWBkZhtcH/meta")},
		{link: newLink(t, "/qfab/hq__2cwnvxLFFeFy6jNxzLeKBG43Xo4HKHab1TxV9k6JS3ByTseM1C1PjFizGAn6pWBkZhtcH/meta/some/path")},
		{link: newLink(t, "/qfab/hq__2cwnvxLFFeFy6jNxzLeKBG43Xo4HKHab1TxV9k6JS3ByTseM1C1PjFizGAn6pWBkZhtcH/files/some/path#10-19")},
	}
	t.Run("without-link-properties", func(t *testing.T) {
		for _, test := range tests {
			t.Run(test.link.String(), func(t *testing.T) {
				runTests(t, *test.link)
			})
		}
	})
	t.Run("with-link-properties", func(t *testing.T) {
		for _, test := range tests {
			test.link.Props = maputil.From("k1", "v1", "k2", "v2")
			t.Run(test.link.String(), func(t *testing.T) {
				runTests(t, *test.link)
			})
		}
	})
}

func newLink(t *testing.T, linkString string) *link.Link {
	val, err := link.FromString(linkString)
	require.NoError(t, err)
	return val
}

func runTests(t *testing.T, val interface{}) {
	{ // simple
		res := encodeDecode(val, t)
		require.EqualValues(t, val, res)
	}

	{ // wrapped
		val = interface{}(maputil.From("value", val))
		res := encodeDecode(val, t)
		require.EqualValues(t, val, res)
	}
}

func encodeDecode(val interface{}, t *testing.T) interface{} {
	buf := &bytes.Buffer{}

	err := codec.Encoder(buf).Encode(val)
	require.NoError(t, err)

	fmt.Println(hex.EncodeToString(buf.Bytes()[7:]))

	var res interface{}
	err = codec.Decoder(buf).Decode(&res)
	require.NoError(t, err)
	return res
}
