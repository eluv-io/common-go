package io

import (
	"io"
	"net/url"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/eluv-io/errors-go"
)

func NewSrtSink(url *url.URL) PacketSink {
	return &srtSink{url: url, urlStr: url.String()}
}

type srtSink struct {
	url    *url.URL
	urlStr string
}

func (s *srtSink) URL() *url.URL {
	return s.url
}

func (s *srtSink) Name() string {
	return s.urlStr
}

func (s *srtSink) Open() (io.WriteCloser, error) {
	connect, err := srtOpen(s.urlStr)
	if err != nil {
		return nil, err
	}
	dw := &DeferredWriter{}
	go func() {
		for {
			conn, err := connect()
			if err != nil {
				// log.Warn("srt connect error", err)
				wc := io.WriteCloser(&ErrorWriter{err: errors.E("srt connect error", errors.K.Invalid.Default(), err)})
				dw.writer.Store(&wc)
				time.Sleep(time.Second)
				continue
			}
			wc := io.WriteCloser(conn)
			dw.writer.Store(&wc)
			break
		}
	}()

	return dw, nil
}

// ---------------------------------------------------------------------------------------------------------------------

type DeferredWriter struct {
	writer atomic.Pointer[io.WriteCloser]
}

func (d *DeferredWriter) Write(p []byte) (n int, err error) {
	w := d.writer.Load()
	if w != nil {
		return (*w).Write(p)
	}
	return 0, errors.E("srt sink not yet connected", errors.K.IO, syscall.ECONNREFUSED)
}

func (d *DeferredWriter) Close() error {
	w := d.writer.Load()
	if w != nil {
		return (*w).Close()
	}
	return nil
}

// ---------------------------------------------------------------------------------------------------------------------

type ErrorWriter struct {
	err error
}

func (e *ErrorWriter) Write([]byte) (n int, err error) {
	return 0, e.err
}

func (e *ErrorWriter) Close() error {
	return nil
}
