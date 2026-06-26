package io

import (
	"io"
	"net"
	"net/url"

	"golang.org/x/net/ipv4"

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
	e := errors.Template("udpSink.Open", errors.K.IO, "url", s.url)

	liveUrl, err := ParseLiveUrl(s.url)
	if err != nil {
		return nil, e(err)
	}

	var conn *net.UDPConn
	if liveUrl.Multicast {
		var iface *net.Interface
		if liveUrl.LocalAddr != nil {
			iface, err = interfaceByIP(liveUrl.LocalAddr)
			if err != nil {
				return nil, e(err)
			}
		}

		conn, err = net.DialUDP("udp", nil, liveUrl.Addr)
		if err != nil {
			return nil, e(err)
		}
		p := ipv4.NewPacketConn(conn)

		defer func() {
			if err != nil {
				errors.Log(conn.Close, log.Warn)
			}
		}()

		if iface != nil {
			err = p.SetMulticastInterface(iface)
			if err != nil {
				return nil, e(err)
			}
		}
		err = p.SetMulticastTTL(liveUrl.TTL)
		if err != nil {
			return nil, e(err)
		}
		err = p.SetMulticastLoopback(liveUrl.Loopback)
		if err != nil {
			return nil, e(err)
		}
	} else {
		// Parse the UDP address
		udpAddr, err := net.ResolveUDPAddr("udp", s.url.Host)
		if err != nil {
			return nil, e(err)
		}

		log.Debug("udp connect", "addr", udpAddr)

		// Connect to the UDP address to send data
		conn, err = net.DialUDP("udp", nil, udpAddr)
		if err != nil {
			return nil, e(err)
		}
	}

	return udpConn{conn}, nil
}

// udpConn wraps a UDP connection so it can report ConnStats (StatsReporter). It is used by both the UDP
// sink and source: for a sink the remote address is the dialed destination, while for a (connectionless)
// source listening socket only the local listen address is available. UDP has no protocol-level stats,
// so SRT is left nil.
type udpConn struct {
	*net.UDPConn
}

func (c udpConn) ConnStats(bool) ConnStats {
	cs := ConnStats{}
	if addr := c.RemoteAddr(); addr != nil {
		cs.RemoteAddr = addr.String()
	}
	if addr := c.LocalAddr(); addr != nil {
		cs.LocalAddr = addr.String()
	}
	return cs
}
