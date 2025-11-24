package io

import (
	"io"
	"net"
	"net/url"

	"github.com/eluv-io/errors-go"
)

func NewUdpSource(url *url.URL) PacketSource {
	return &udpSource{url: url, name: url.String()}
}

type udpSource struct {
	url  *url.URL
	name string
}

func (s *udpSource) URL() *url.URL {
	return s.url
}

func (s *udpSource) Name() string {
	return s.name
}

func (s *udpSource) Open() (io.ReadCloser, error) {
	e := errors.Template("udpSource.Open", errors.K.IO, "url", s.url)

	// Parse the UDP address
	udpAddr, err := net.ResolveUDPAddr("udp", s.url.Host)
	if err != nil {
		return nil, e(err)
	}

	// Listen on the UDP address to receive data
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, e(err)
	}

	return conn, nil
}
