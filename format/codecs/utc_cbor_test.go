package codecs_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/utc-go"
)

var oneBillion = time.Unix(1000000000, 0)

func TestCBOR(t *testing.T) {
	ut := utc.New(oneBillion)

	buf := &bytes.Buffer{}

	err := cborCodec.Encoder(buf).Encode(ut)
	require.NoError(t, err)

	var utDecoded utc.UTC
	err = cborCodec.Decoder(buf).Decode(&utDecoded)
	require.NoError(t, err)

	require.Equal(t, ut.String(), utDecoded.String())
	assertTimezone(t, utDecoded)
}

func TestWrappedCBOR(t *testing.T) {
	ut := utc.New(oneBillion)

	wrapper := Wrapper{
		UTC: ut,
	}

	buf := &bytes.Buffer{}

	err := cborCodec.Encoder(buf).Encode(wrapper)
	require.NoError(t, err)

	var wrapperDecoded Wrapper
	err = cborCodec.Decoder(buf).Decode(&wrapperDecoded)
	require.NoError(t, err)

	require.Equal(t, ut.String(), wrapperDecoded.UTC.String())
}

func TestGenericCBOR(t *testing.T) {
	ut := utc.New(oneBillion)
	m := map[string]interface{}{
		"billion": ut,
	}

	buf := &bytes.Buffer{}

	err := cborCodec.Encoder(buf).Encode(m)
	require.NoError(t, err)

	var mDecoded interface{}
	err = cborCodec.Decoder(buf).Decode(&mDecoded)
	require.NoError(t, err)

	genDecoded := mDecoded.(map[string]interface{})["billion"]
	spew.Dump(genDecoded)
	utDecoded, ok := genDecoded.(utc.UTC)
	require.True(t, ok, genDecoded)
	require.Equal(t, ut.String(), utDecoded.String(), ok)
}

type Wrapper struct {
	UTC utc.UTC
}

func assertTimezone(t *testing.T, val utc.UTC) {
	zone, offset := val.Zone()
	require.Equal(t, 0, offset)
	require.Equal(t, "UTC", zone)
}
