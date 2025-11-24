package io

import (
	"io"
	"net"
	"net/url"

	"github.com/eluv-io/errors-go"
)

func NewUdpSink(url *url.URL) PacketSink {
	return &udpSink{url: url, name: url.String()}
}

type udpSink struct {
	url  *url.URL
	name string
}

func (s *udpSink) URL() *url.URL {
	return s.url
}

func (s *udpSink) Name() string {
	return s.name
}

func (s *udpSink) Open() (io.WriteCloser, error) {
	e := errors.Template("udpSource.Open", errors.K.IO, "url", s.url)

	// Parse the UDP address
	udpAddr, err := net.ResolveUDPAddr("udp", s.url.Host)
	if err != nil {
		return nil, e(err)
	}

	// Listen on the UDP address to receive data
	// conn, err := net.ListenUDP("udp", udpAddr)

	// Connect to the UDP address to send data
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, e(err)
	}

	return conn, nil
}
