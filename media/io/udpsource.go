package io

import (
	"io"
	"net"
	"net/url"

	"golang.org/x/net/ipv4"

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

		log.Trace("open live URL multicast", "group", liveUrl.Group, "localaddr", liveUrl.LocalAddr, "if", iface)

		// Commonly ListenMulticastUDP() will bind to all interfaces and join the
		// specified group.  This causes problem when we have streams on different multicast groups
		// but same ports - in this case all sockets will read packets from all groups on that port.
		conn, err = net.ListenUDP("udp", liveUrl.Group)
		if err != nil {
			return nil, e(err, "reason", "failed to listen on multicast group")
		}

		err = setPlatformOptions(conn)
		if err != nil {
			return nil, e(err, "reason", "failed to set platform options")
		}

		p := ipv4.NewPacketConn(conn)
		if err := p.JoinGroup(iface, &net.UDPAddr{IP: liveUrl.Group.IP}); err != nil {
			errors.Log(conn.Close, log.Warn)
			return nil, e(err, "reason", "failed to join multicast group")
		}
		log.Trace("listening on UDP multicast", "group", liveUrl.Group, "localaddr", liveUrl.LocalAddr, "interface", iface)
	} else {
		bindAddr := liveUrl.Addr
		if liveUrl.LocalAddr != nil {
			bindAddr = &net.UDPAddr{
				IP:   liveUrl.LocalAddr,
				Port: liveUrl.Port,
			}
		}
		conn, err = net.ListenUDP("udp", bindAddr)
		if err != nil {
			return nil, e(err)
		}
		log.Trace("listening on UDP address", "addr", bindAddr)
	}

	return conn, nil
}
