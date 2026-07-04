package io

import (
	"io"
	"net/url"
	"sync"
	"sync/atomic"
	"syscall"

	srt "github.com/datarhei/gosrt"

	"github.com/eluv-io/errors-go"
)

func NewSrtSource(url *url.URL) PacketSource {
	return &srtSource{url: url, urlStr: url.String()}
}

type srtSource struct {
	url    *url.URL
	urlStr string
}

func (s *srtSource) URL() *url.URL {
	return s.url
}

func (s *srtSource) Name() string {
	return s.urlStr
}

func (s *srtSource) Open() (reader io.ReadCloser, err error) {
	settings, err := srtParse(s.urlStr)
	if err != nil {
		return nil, err
	}

	if !settings.modeListen {
		// caller mode: dial the remote server synchronously
		return settings.dial()
	}

	// Listener mode: bind the listener eagerly (rather than inside the accept goroutine) so that DeferredReader.Close
	// can close it to interrupt a pending Accept. This lets a caller (e.g. the avpipe NetReader on cancel/shutdown)
	// unblock a Read that is waiting for a source that never (re)connects, instead of hanging until a connection
	// arrives or a timeout elapses.
	listener, err := settings.listen()
	if err != nil {
		return nil, err
	}

	dr := &DeferredReader{
		waitReader: make(chan struct{}),
		listener:   listener,
	}
	go func() {
		// Always signal waitReader once the accept attempt completes (success or failure) so a pending Read unblocks:
		// on success it reads from the connection, on failure it returns the stored ErrorReader's error.
		defer close(dr.waitReader)

		var rc io.ReadCloser
		conn, err := settings.accept(listener)
		if err != nil {
			listener.Close()
			rc = &ErrorReader{err: errors.E("srt listen error", errors.K.Invalid.Default(), err)}
		} else {
			rc = conn
		}

		// Publish the reader under the lock and, if Close already ran, close the just-established connection ourselves
		// so it (and its stats goroutine) is not leaked.
		dr.mu.Lock()
		closed := dr.closed
		dr.reader.Store(&rc)
		dr.mu.Unlock()
		if closed {
			errors.Ignore(rc.Close)
		}
	}()

	return dr, nil
}

// ---------------------------------------------------------------------------------------------------------------------

type DeferredReader struct {
	waitReader chan struct{}
	reader     atomic.Pointer[io.ReadCloser]

	// listener is set in listener mode; closing it interrupts a pending Accept in the connect goroutine so that Close
	// returns promptly even when no source has connected yet.
	listener srt.Listener

	// mu guards closed and serializes it against the connect goroutine's reader.Store so that a connection established
	// concurrently with Close is always torn down (see Open and Close).
	mu     sync.Mutex
	closed bool
}

func (d *DeferredReader) Read(p []byte) (n int, err error) {
	<-d.waitReader
	w := d.reader.Load()
	if w != nil {
		return (*w).Read(p)
	}
	return 0, errors.E("srt source not yet connected", errors.K.IO, syscall.ECONNREFUSED)
}

func (d *DeferredReader) Close() error {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return nil
	}
	d.closed = true
	w := d.reader.Load()
	d.mu.Unlock()

	// Close the listener first so a pending Accept in the connect goroutine unblocks and the goroutine finishes
	// (closing waitReader). If a connection was already established, close it too; if it is established concurrently
	// with this call, the connect goroutine closes it (it observes d.closed).
	if d.listener != nil {
		d.listener.Close()
	}
	if w != nil {
		return (*w).Close()
	}
	return nil
}

// ConnStats implements StatsReporter by delegating to the underlying connection once it is connected. Returns a zero
// ConnStats while no connection has been established yet.
func (d *DeferredReader) ConnStats(details bool) ConnStats {
	if w := d.reader.Load(); w != nil {
		if r, ok := (*w).(StatsReporter); ok {
			return r.ConnStats(details)
		}
	}
	return ConnStats{}
}

// ---------------------------------------------------------------------------------------------------------------------

type ErrorReader struct {
	err error
}

func (e *ErrorReader) Read([]byte) (n int, err error) {
	return 0, e.err
}

func (e *ErrorReader) Close() error {
	return nil
}
