package io

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/testutil"
)

func TestSourceSink(t *testing.T) {
	log.SetDebug()

	tests := []struct {
		proto          string
		connectionLess bool
	}{
		{"udp", true},
		{"rtp", true},
		{"srt", false},
	}
	for _, test := range tests {
		t.Run(test.proto, func(t *testing.T) {
			for _, host := range []string{
				"127.0.0.1",
				"[::1]", // ==> not sure it's working on github actions
				// "localhost",  ==> doesn't work for srt!
			} {
				t.Run(host, func(t *testing.T) {
					testSourceSink(t, host, test.proto)
				})
			}
		})
	}
}

func TestSourceSinkMulticast(t *testing.T) {
	port, err := testutil.FreePort()
	require.NoError(t, err)
	testSourceSinkUrl(t, fmt.Sprintf("udp://239.255.0.1:%d?localaddr=127.0.0.1", port), fmt.Sprintf("udp://239.255.0.1:%d?localaddr=127.0.0.1&loopback", port))
}

func testSourceSink(t *testing.T, host, proto string) {
	port, err := testutil.FreePort()
	require.NoError(t, err)
	testSourceSinkUrl(t, fmt.Sprintf("%s://%s:%d?mode=listener", proto, host, port), fmt.Sprintf("%s://%s:%d", proto, host, port))
}

func testSourceSinkUrl(t *testing.T, src, snk string) {
	// create the source
	source, err := CreatePacketSource(src)
	require.NoError(t, err)
	reader, err := source.Open()
	require.NoError(t, err)
	defer log.Call(reader.Close, "close source")

	// allow some time for the listener to start
	time.Sleep(100 * time.Millisecond)

	// connect the sink
	sink, err := CreatePacketSink(snk)
	require.NoError(t, err)
	writer, err := sink.Open()
	require.NoError(t, err)
	defer log.Call(writer.Close, "close sink")

	// write to the sink
	n, err := writer.Write([]byte{1, 2, 3})
	require.NoError(t, err)
	require.Equal(t, 3, n)

	// read from the source
	packet := make([]byte, 1024)
	n, err = reader.Read(packet)
	require.NoError(t, err)
	require.Equal(t, 3, n)

	require.Equal(t, []byte{1, 2, 3}, packet[:n])
}

func TestSourceCreationErrors(t *testing.T) {
	tests := []struct {
		url       string
		createErr string
		openErr   string
	}{
		{"udp://invalid_address:8080", "", "no such host"},
		{"rtp://invalid_address:8080", "", "no such host"},
		{"srt://invalid_address:8080", "", "failed dialing"},
		{"http://invalid_address:8080", "unsupported protocol", ""},
	}

	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			src, err := CreatePacketSource(test.url)
			if test.createErr != "" {
				require.ErrorContains(t, err, test.createErr)
				return
			}
			require.NoError(t, err)
			_, err = src.Open()
			// github build returns different errors...
			// require.ErrorContains(t, err, test.openErr)
			require.Error(t, err)
		})
	}
}

func TestSinkCreationErrors(t *testing.T) {
	tests := []struct {
		url       string
		createErr string
		openErr   string
	}{
		{"udp://invalid_address:8080", "", "no such host"},
		{"rtp://invalid_address:8080", "", "no such host"},
		{"srt://invalid_address:8080", "", "failed dialing"},
		{"http://invalid_address:8080", "unsupported protocol", ""},
	}

	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			sink, err := CreatePacketSink(test.url)
			if test.createErr != "" {
				require.ErrorContains(t, err, test.createErr)
				return
			}
			require.NoError(t, err)
			_, err = sink.Open()
			// github build returns different errors...
			// require.ErrorContains(t, err, test.openErr)
			require.Error(t, err)
		})
	}
}
