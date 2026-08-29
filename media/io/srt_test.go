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

	// both the source reader and the sink writer should report connection stats
	for name, rc := range map[string]any{"source": reader, "sink": writer} {
		sr, ok := rc.(StatsReporter)
		require.True(t, ok, "%s does not implement StatsReporter", name)
		var stats ConnStats
		sr.ConnStats(&stats, true)
		// the local address is always known once connected; the source listens on it and the sink
		// binds a local socket to it
		require.NotEmpty(t, stats.LocalAddr, "%s local address", name)
	}
}

// TestSrtSourceCloseUnblocksAccept verifies that closing an SRT source in listener mode promptly interrupts a pending
// Accept (and the Read waiting on it) even when no peer ever connects. This is the shutdown path exercised when a live
// recorder is cancelled while waiting for a source to (re)connect.
func TestSrtSourceCloseUnblocksAccept(t *testing.T) {
	log.SetDebug()

	port, err := testutil.FreePort()
	require.NoError(t, err)
	url := fmt.Sprintf("srt://127.0.0.1:%d?mode=listener", port)

	source, err := CreatePacketSource(url)
	require.NoError(t, err)
	reader, err := source.Open()
	require.NoError(t, err)

	// Allow the listener to start and the accept to block waiting for a connection that never arrives.
	time.Sleep(200 * time.Millisecond)

	// A Read blocks until a source connects or the reader is closed.
	readDone := make(chan error, 1)
	go func() {
		_, rerr := reader.Read(make([]byte, 1024))
		readDone <- rerr
	}()

	// Confirm the Read is genuinely blocked while no source is connected.
	select {
	case rerr := <-readDone:
		t.Fatalf("Read returned before any source connected or Close: %v", rerr)
	case <-time.After(200 * time.Millisecond):
	}

	// Closing the source must promptly interrupt the pending Accept.
	closeDone := make(chan error, 1)
	go func() { closeDone <- reader.Close() }()
	select {
	case cerr := <-closeDone:
		require.NoError(t, cerr)
	case <-time.After(time.Second):
		t.Fatal("Close did not return promptly after cancel")
	}

	// ...and the blocked Read must unblock with an error rather than hang.
	select {
	case rerr := <-readDone:
		require.Error(t, rerr)
	case <-time.After(time.Second):
		t.Fatal("Read did not unblock promptly after Close")
	}
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
