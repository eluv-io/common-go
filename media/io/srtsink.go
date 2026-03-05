package io

import (
	"io"
	"net/url"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/datarhei/gosrt/packet"

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
	connect, modeListen, err := srtOpen(s.urlStr)
	if err != nil {
		return nil, err
	}

	if !modeListen {
		return connect()
	}

	dw := &DeferredWriter{}
	go func() {
		for {
			conn, err := connect()
			if err != nil {
				// log.Warn("srt connect error", err)
				wc := srtWriter(&ErrorWriter{err: errors.E("srt connect error", errors.K.Invalid.Default(), err)})
				dw.writer.Store(&wc)
				time.Sleep(time.Second)
				continue
			}
			wc := srtWriter(conn)
			dw.writer.Store(&wc)
			break
		}
	}()

	return dw, nil
}

// ---------------------------------------------------------------------------------------------------------------------

type srtWriter interface {
	io.WriteCloser
	WritePacket(p packet.Packet) error
}

// ---------------------------------------------------------------------------------------------------------------------

type DeferredWriter struct {
	writer atomic.Pointer[srtWriter]
}

func (d *DeferredWriter) Write(p []byte) (n int, err error) {
	w := d.writer.Load()
	if w != nil {
		return (*w).Write(p)
	}
	return 0, errors.E("srt sink not yet connected", errors.K.IO, syscall.ECONNREFUSED)
}

func (d *DeferredWriter) WritePacket(p packet.Packet) error {
	w := d.writer.Load()
	if w != nil {
		return (*w).WritePacket(p)
	}
	return errors.E("srt sink not yet connected", errors.K.IO, syscall.ECONNREFUSED)
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

func (e *ErrorWriter) WritePacket(packet.Packet) error {
	return e.err
}

func (e *ErrorWriter) Close() error {
	return nil
}
