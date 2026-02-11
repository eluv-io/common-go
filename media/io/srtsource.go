package io

import (
	"encoding/json"
	"io"
	"net/url"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

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
	connect, modeListen, err := srtOpen(s.urlStr)
	if err != nil {
		return nil, err
	}

	if !modeListen {
		return connect()
	}

	dr := &DeferredReader{
		waitReader: make(chan struct{}),
	}
	go func() {
		var rc io.ReadCloser
		conn, err := connect()
		if err != nil {
			// log.Warn("srt connect error", err)
			rc = io.ReadCloser(&ErrorReader{err: errors.E("srt listen error", errors.K.Invalid.Default(), err)})
			dr.reader.Store(&rc)
			return
		} else {
			rc = newSrtReader(conn)
		}
		dr.reader.Store(&rc)
		close(dr.waitReader)
	}()

	return dr, nil
}

// ---------------------------------------------------------------------------------------------------------------------

type DeferredReader struct {
	waitReader chan struct{}
	reader     atomic.Pointer[io.ReadCloser]
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
	w := d.reader.Load()
	if w != nil {
		return (*w).Close()
	}
	return nil
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

// ---------------------------------------------------------------------------------------------------------------------

type srtReader struct {
	srt.Conn
	done chan bool
	once sync.Once
}

func newSrtReader(conn srt.Conn) io.ReadCloser {
	done := make(chan bool, 1)
	go func() {
		addr := conn.RemoteAddr().String()
		stats := &srt.Statistics{}
		report := func() {
			conn.Stats(stats)
			res, _ := json.Marshal(stats)
			log.Debug("srt stats", "addr", addr, "stats", string(res))
		}
		ticker := time.NewTicker(time.Second * 5)
		for {
			select {
			case <-ticker.C:
				report()
			case <-done:
				report()
				return
			}
		}
	}()
	return &srtReader{Conn: conn, done: done}
}

func (r *srtReader) Close() error {
	r.once.Do(func() {
		close(r.done)
	})
	return r.Conn.Close()
}
