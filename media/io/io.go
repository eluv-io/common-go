package io

import (
	"io"
	"net/url"
	"strings"

	"github.com/eluv-io/common-go/util/ioutil"
	"github.com/eluv-io/errors-go"
	elog "github.com/eluv-io/log-go"
)

var log = elog.Get("/eluvio/media/io")

type PacketSource interface {
	Name() string
	URL() *url.URL
	Open() (io.ReadCloser, error)
}

type PacketSink interface {
	Name() string
	URL() *url.URL
	Open() (io.WriteCloser, error)
}

// ---------------------------------------------------------------------------------------------------------------------

type NoopSink struct{}

func (n *NoopSink) Name() string {
	return "noop"
}

func (n *NoopSink) URL() *url.URL {
	u, _ := url.Parse("noop:")
	return u
}

func (n *NoopSink) Open() (io.WriteCloser, error) {
	return ioutil.NopWriter(), nil
}

// ---------------------------------------------------------------------------------------------------------------------

func CreatePacketSource(sourceUrl string) (packetSource PacketSource, err error) {
	u, err := url.Parse(sourceUrl)
	if err != nil {
		return nil, err
	}
	switch {
	case u.Scheme == "rtp":
		packetSource = NewUdpSource(u)
	case u.Scheme == "udp":
		packetSource = NewUdpSource(u)
	case strings.HasPrefix(u.Scheme, "srt"):
		packetSource = NewSrtSource(u)
	case strings.HasPrefix(u.Scheme, "file"):
		packetSource = NewFileSource(u)
	default:
		err = errors.E("createPacketSource", errors.K.Invalid,
			"source", sourceUrl,
			"reason", "unsupported protocol, expecting udp|rtp|srt|file",
		)
	}
	return packetSource, err
}

func CreatePacketSink(sinkUrl string) (packetSink PacketSink, err error) {
	u, err := url.Parse(sinkUrl)
	if err != nil {
		return nil, err
	}
	switch {
	case strings.HasPrefix(u.Scheme, "srt"):
		packetSink = NewSrtSink(u)
	case strings.HasPrefix(u.Scheme, "udp"):
		packetSink = NewUdpSink(u)
	case strings.HasPrefix(u.Scheme, "rtp"):
		packetSink = NewUdpSink(u)
	case strings.HasPrefix(u.Scheme, "file"):
		packetSink = NewFileSink(u)
	default:
		err = errors.E("createPacketSink", errors.K.Invalid,
			"sink", sinkUrl,
			"reason", "unsupported protocol, expecting udp|rtp|srt",
		)
	}
	return packetSink, err
}
