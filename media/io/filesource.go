package io

import (
	"bytes"
	"io"
	"net/url"
	"os"

	"github.com/eluv-io/errors-go"
)

func NewFileSource(url *url.URL) PacketSource {
	return &fileSource{url: url, name: url.String()}
}

type fileSource struct {
	url  *url.URL
	name string
}

func (s *fileSource) URL() *url.URL {
	return s.url
}

func (s *fileSource) Name() string {
	return s.name
}

func (s *fileSource) Open() (io.ReadCloser, error) {
	e := errors.Template("fileSource.Open", errors.K.IO, "url", s.url)

	f, err := os.Open(s.url.Path)
	if err != nil {
		return nil, e(err)
	}

	return &fileReader{f: f}, nil
}

type fileReader struct {
	f       *os.File
	hasRead bool
}

func (f *fileReader) Read(p []byte) (n int, err error) {
	n, err = f.f.Read(p)
	if !f.hasRead {
		if len(p) < 7 {
			return 0, errors.E("fileReader.Read", errors.K.IO, "reason", "read buffer too small")
		}
		if n >= 7 && bytes.Equal(p[:7], []byte{0x06, 0x05, 0x2f, 0x72, 0x61, 0x77, 0x0a}) {
			// live part header/preamble (varint, varint, /, r, a, w, \n) ... strip!
			copy(p, p[7:n])
			n -= 7
		}

		f.hasRead = true
	}
	return n, err
}

func (f *fileReader) Close() error {
	return f.f.Close()
}
