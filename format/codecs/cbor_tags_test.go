package codecs_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/utc-go"

	"github.com/eluv-io/common-go/format"
	"github.com/eluv-io/common-go/format/codecs"
	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/link"
	"github.com/eluv-io/common-go/util/maputil"
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

func TestUTCTag(t *testing.T) {
	val := utc.Now().StripMono()
	runTests(t, val)
	runTests(t, utc.New(time.Date(9999, 12, 12, 23, 59, 59, 999999999, time.UTC)))
	runTests(t, utc.New(time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC)))
}

func newLink(t *testing.T, linkString string) *link.Link {
	val, err := link.FromString(linkString)
	require.NoError(t, err)
	return val
}

func runTests(t *testing.T, val interface{}) {
	{ // simple
		res := encodeDecode(val, t)
		require.Equal(t, val, res, "expected %s actual %s", val, res)
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

	bts := buf.Bytes()
	fmt.Println(fmt.Sprintf("%T", val), hex.EncodeToString(bts[bytes.IndexByte(bts, '\n')+1:]))

	var res interface{}
	err = codec.Decoder(buf).Decode(&res)
	require.NoError(t, err)
	return res
}
