package io

import (
	"io"
	"net/url"
	"os"

	"github.com/eluv-io/errors-go"
)

func NewFileSink(url *url.URL) PacketSink {
	return &fileSink{url: url, name: url.String()}
}

type fileSink struct {
	url  *url.URL
	name string
}

func (s *fileSink) URL() *url.URL {
	return s.url
}

func (s *fileSink) Name() string {
	return s.name
}

func (s *fileSink) Open() (io.WriteCloser, error) {
	e := errors.Template("fileSink.Open", errors.K.IO, "url", s.url)

	file, err := os.OpenFile(s.url.Path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return nil, e(err)
	}

	return file, nil
}
