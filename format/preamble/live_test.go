package preamble

import (
	"bytes"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestLiveRoundTrip(t *testing.T) {
	timeouts := &LiveTimeouts{Init: time.Minute, Idle: 10 * time.Minute, Transfer: 4 * time.Hour}
	data, format, err := EncodeLive(timeouts, []byte("user"), "/user-fmt")
	require.NoError(t, err)
	require.Equal(t, LiveFormat, format)

	// through a part preamble
	buf := &bytes.Buffer{}
	_, err = Write(buf, data, format)
	require.NoError(t, err)
	pdata, pformat, _, err := Read(bytes.NewReader(buf.Bytes()), true)
	require.NoError(t, err)

	l, err := DecodeLive(pdata, pformat)
	require.NoError(t, err)
	require.Equal(t, uint(LiveVersion), l.Version)
	require.Equal(t, timeouts, l.Timeouts)
	require.Equal(t, "/user-fmt", l.Format)
	require.Equal(t, []byte("user"), l.Data)
}

func TestLiveWithoutTimeouts(t *testing.T) {
	for _, timeouts := range []*LiveTimeouts{nil, {}} {
		data, format, err := EncodeLive(timeouts, []byte("user"), "/raw")
		require.NoError(t, err)
		require.Equal(t, []byte("user"), data)
		require.Equal(t, "/raw", format)

		l, err := DecodeLive(data, format)
		require.NoError(t, err)
		require.True(t, l.Timeouts.IsZero())
		require.Equal(t, "/raw", l.Format)
		require.Equal(t, []byte("user"), l.Data)
	}
}

func TestLivePartialTimeouts(t *testing.T) {
	data, format, err := EncodeLive(&LiveTimeouts{Idle: time.Hour}, nil, "")
	require.NoError(t, err)
	l, err := DecodeLive(data, format)
	require.NoError(t, err)
	require.Equal(t, &LiveTimeouts{Idle: time.Hour}, l.Timeouts)
	require.Empty(t, l.Data)
}

func TestLiveUnknownFields(t *testing.T) {
	enc, err := cbor.Marshal(map[int]any{
		1: 2,
		2: map[int]any{2: int64(time.Hour), 9: "future timeout"},
		4: []byte("user"),
		9: "future field",
	})
	require.NoError(t, err)

	l, err := DecodeLive(enc, LiveFormat)
	require.NoError(t, err)
	require.Equal(t, uint(2), l.Version)
	require.Equal(t, &LiveTimeouts{Idle: time.Hour}, l.Timeouts)
	require.Equal(t, []byte("user"), l.Data)
}

func TestLiveInvalid(t *testing.T) {
	_, _, err := EncodeLive(&LiveTimeouts{Idle: -time.Second}, nil, "")
	require.Error(t, err)

	_, err = DecodeLive([]byte{0xff, 0x00}, LiveFormat)
	require.Error(t, err)
}
