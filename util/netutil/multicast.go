package netutil

import (
	"context"
	"net"
	"strings"

	"golang.org/x/net/ipv4"

	"github.com/eluv-io/errors-go"
)

type MulticastReceiver struct {
	conn  *ipv4.PacketConn
	group net.IP
	ip    net.IP
	buf   []byte
}

func NewMulticastReceiver(ctx context.Context, multicastAddr string, localAddr string) (*MulticastReceiver, error) {
	e := errors.Template("MulticastReceiver", errors.K.IO.Default())

	multicastIP, port, err := net.SplitHostPort(multicastAddr)
	if err != nil {
		return nil, e(errors.K.Invalid, err, "addr", multicastAddr)
	}
	localIP, _, err := net.SplitHostPort(localAddr)
	if err != nil {
		if strings.Contains(err.Error(), "missing port in address") {
			localIP = localAddr
		} else {
			return nil, e(errors.K.Invalid, err, "addr", localAddr)
		}
	}

	group := net.ParseIP(multicastIP)
	ip := net.ParseIP(localIP)
	addr := net.JoinHostPort(ip.String(), port)
	iface, err := findInterfaceByIP(ip)
	if err != nil {
		return nil, e(err, "addr", localAddr)
	}

	var c net.PacketConn
	if ctx == nil {
		c, err = net.ListenPacket("udp4", addr)
	} else {
		c, err = (&net.ListenConfig{}).ListenPacket(ctx, "udp4", addr)
	}
	if err != nil {
		return nil, e(err, "addr", localAddr)
	}

	conn := ipv4.NewPacketConn(c)
	err = conn.SetMulticastLoopback(false)
	if err == nil {
		err = conn.SetControlMessage(ipv4.FlagDst, true)
		if err == nil {
			err = conn.JoinGroup(iface, &net.UDPAddr{IP: group})
		}
	}
	if err != nil {
		return nil, e(err, "addr", multicastAddr)
	}

	return &MulticastReceiver{
		conn:  conn,
		group: group,
		ip:    ip,
		buf:   make([]byte, 66507), // Maximum packet payload size
	}, nil
}

func (r *MulticastReceiver) ReadPacket() ([]byte, bool, string, error) {
	e := errors.Template("MulticastReceiver.ReadPacket", errors.K.IO)
	if r.conn == nil {
		return nil, false, "", e("reason", "closed")
	}
	for {
		n, cm, src, err := r.conn.ReadFrom(r.buf)
		if err != nil {
			return nil, false, "", e(err)
		} else if !cm.Dst.Equal(r.group) && !cm.Dst.Equal(r.ip) {
			continue
		}
		return r.buf[:n], cm.Dst.IsMulticast(), src.String(), nil
	}
}

func (r *MulticastReceiver) Close() error {
	var err error
	if r.conn != nil {
		err = r.conn.Close()
		r.conn = nil
	}
	return err
}

func NewMulticastSender(ctx context.Context, multicastAddr string, maxPacketSize ...int) (*MulticastSender, error) {
	e := errors.Template("MulticastSender", errors.K.IO.Default(), "addr", multicastAddr)
	addr, err := net.ResolveUDPAddr("udp4", multicastAddr)
	if err != nil {
		return nil, e(errors.K.Invalid, err)
	}
	var conn net.Conn
	if ctx == nil {
		conn, err = net.DialUDP("udp4", nil, addr)
	} else {
		conn, err = (&net.Dialer{}).DialContext(ctx, "udp4", addr.String())
	}
	if err != nil {
		return nil, e(err)
	}
	mtu := 1472 // Realistic MTU payload size
	if len(maxPacketSize) > 0 && maxPacketSize[0] > 0 {
		mtu = maxPacketSize[0]
	}
	return &MulticastSender{
		conn: conn,
		mtu:  mtu,
	}, nil
}

type MulticastSender struct {
	conn net.Conn
	mtu  int
}

func (s *MulticastSender) WritePacket(buf []byte) error {
	e := errors.Template("MulticastSender.WritePacket", errors.K.IO.Default())
	if s.conn == nil {
		return e("reason", "closed")
	} else if len(buf) > s.mtu {
		return e(errors.K.Invalid, "reason", "packet exceeds maximum size", "size", len(buf), "max_size", s.mtu)
	}
	_, err := s.conn.Write(buf)
	return e.IfNotNil(err)
}

func (s *MulticastSender) Close() error {
	var err error
	if s.conn != nil {
		err = s.conn.Close()
		s.conn = nil
	}
	return err
}

func (s *MulticastSender) MaxPacketSize() int {
	return s.mtu
}

func findInterfaceByIP(ip net.IP) (*net.Interface, error) {
	e := errors.Template("findInterfaceByIP", errors.K.Invalid, "ip", ip)
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, e(err)
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, e(err)
		}
		for _, addr := range addrs {
			ipaddr, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				continue
			} else if ipaddr.Equal(ip) {
				return &iface, nil
			}
		}
	}
	return nil, e("reason", "interface not found")
}
