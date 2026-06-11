package netutil

import (
	"context"
	"net"
	"strings"
	"time"

	"golang.org/x/net/ipv4"

	"github.com/eluv-io/errors-go"
)

var controlFlags = ipv4.FlagDst

type MulticastReceiver struct {
	conn      *ipv4.PacketConn
	stop      func() bool
	multicast net.IP
	unicast   net.IP
	buf       []byte
}

// NewMulticastReceiver creates a MulticastReceiver that joins and listens on the given multicast address, using the
// interface specified by the given interface address. MulticastReceiver reads packets received at both the multicast
// address and the interface address. The multicast address is expected to contain an ip address and port. The interface
// address is expected to contain an ip address; any port specified in the interface address will be ignored.
func NewMulticastReceiver(ctx context.Context, multicastAddr string, interfaceAddr string) (*MulticastReceiver, error) {
	e := errors.Template("MulticastReceiver", errors.K.IO.Default())

	multicastIP, port, err := net.SplitHostPort(multicastAddr)
	if err != nil {
		return nil, e(errors.K.Invalid, err, "addr", multicastAddr)
	}
	interfaceIP, _, err := net.SplitHostPort(interfaceAddr)
	if err != nil {
		if strings.Contains(err.Error(), "missing port in address") {
			interfaceIP = interfaceAddr
		} else {
			return nil, e(errors.K.Invalid, err, "addr", interfaceAddr)
		}
	}

	network := "udp4"
	addr := net.JoinHostPort("0.0.0.0", port)
	iface, err := findInterfaceByIP(interfaceIP)
	if err != nil {
		return nil, e(err, network, interfaceAddr)
	}
	group, err := net.ResolveUDPAddr(network, multicastAddr)
	if err != nil {
		return nil, e(errors.K.Invalid, err, "addr", multicastAddr)
	}

	var c net.PacketConn
	stop := func() bool {
		return false
	}
	if ctx == nil {
		c, err = net.ListenPacket(network, addr)
	} else {
		c, err = (&net.ListenConfig{}).ListenPacket(ctx, network, addr)
		if err == nil {
			stop = context.AfterFunc(ctx, func() {
				c.SetReadDeadline(time.Now().Add(time.Second * -1))
			})
		}
	}
	if err != nil {
		return nil, e(err, "addr", multicastAddr)
	}

	conn := ipv4.NewPacketConn(c)
	err = conn.SetControlMessage(controlFlags, true)
	if err == nil {
		err = conn.JoinGroup(iface, group)
	}
	if err != nil {
		return nil, e(err, "addr", multicastAddr)
	}

	return &MulticastReceiver{
		conn:      conn,
		stop:      stop,
		multicast: net.ParseIP(multicastIP),
		unicast:   net.ParseIP(interfaceIP),
		buf:       make([]byte, 66507), // Maximum packet payload size
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
		} else if !cm.Dst.Equal(r.multicast) && !cm.Dst.Equal(r.unicast) {
			continue
		}
		return r.buf[:n], cm.Dst.IsMulticast(), src.String(), nil
	}
}

func (r *MulticastReceiver) Close() error {
	var err error
	if r.conn != nil {
		_ = r.stop()
		err = r.conn.Close()
		r.conn = nil
	}
	return err
}

// NewMulticastSender creates a NewMulticastSender that connects to the given multicast address, using the interface
// specified by the given interface address. MulticastSender sends packets to the multicast address using multicast.
// As a special case, the multicast address is permitted to be an unicast address, in which case unicast will be used.
// Optionally, a multicast TTL may be specified for multicast sends. The multicast address is expected to contain an ip
// address and port. The interface address is expected to contain an ip address; any port specified in the interface
// address will be ignored.
func NewMulticastSender(ctx context.Context, multicastAddr string, interfaceAddr string, multicastTTL ...int) (*MulticastSender, error) {
	e := errors.Template("MulticastSender", errors.K.IO.Default(), "addr", multicastAddr)

	_, _, err := net.SplitHostPort(multicastAddr)
	if err != nil {
		return nil, e(errors.K.Invalid, err, "addr", multicastAddr)
	}
	interfaceIP, _, err := net.SplitHostPort(interfaceAddr)
	if err != nil {
		if strings.Contains(err.Error(), "missing port in address") {
			interfaceIP = interfaceAddr
		} else {
			return nil, e(errors.K.Invalid, err, "addr", interfaceAddr)
		}
	}
	ttl := 1
	if len(multicastTTL) > 0 && multicastTTL[0] > 0 {
		ttl = multicastTTL[0]
	}

	network := "udp4"
	addr := net.JoinHostPort("0.0.0.0", "0")
	iface, err := findInterfaceByIP(interfaceIP)
	if err != nil {
		return nil, e(err, "addr", interfaceAddr)
	}
	dest, err := net.ResolveUDPAddr(network, multicastAddr)
	if err != nil {
		return nil, e(errors.K.Invalid, err, "addr", multicastAddr)
	}

	var c net.PacketConn
	stop := func() bool {
		return false
	}
	if ctx == nil {
		c, err = net.ListenPacket(network, addr)
	} else {
		c, err = (&net.ListenConfig{}).ListenPacket(ctx, network, addr)
		if err == nil {
			stop = context.AfterFunc(ctx, func() {
				c.SetWriteDeadline(time.Now().Add(time.Second * -1))
			})
		}
	}
	if err != nil {
		return nil, e(err, "addr", multicastAddr)
	}

	conn := ipv4.NewPacketConn(c)
	err = conn.SetControlMessage(controlFlags, true)
	if err == nil {
		err = conn.SetMulticastLoopback(false)
		if err == nil {
			err = conn.SetMulticastTTL(ttl)
			if err == nil {
				err = conn.SetMulticastInterface(iface)
			}
		}
	}
	if err != nil {
		return nil, e(err, "addr", multicastAddr)
	}

	return &MulticastSender{
		conn: conn,
		stop: stop,
		dest: dest,
		mtu:  1472, // Realistic MTU payload size
	}, nil
}

type MulticastSender struct {
	conn *ipv4.PacketConn
	stop func() bool
	dest *net.UDPAddr
	mtu  int
}

func (s *MulticastSender) WritePacket(buf []byte) error {
	e := errors.Template("MulticastSender.WritePacket", errors.K.IO.Default())
	if s.conn == nil {
		return e("reason", "closed")
	} else if len(buf) > s.mtu {
		return e(errors.K.Invalid, "reason", "packet exceeds maximum size", "size", len(buf), "max_size", s.mtu)
	}
	_, err := s.conn.WriteTo(buf, nil, s.dest)
	return e.IfNotNil(err)
}

func (s *MulticastSender) Close() error {
	var err error
	if s.conn != nil {
		_ = s.stop()
		err = s.conn.Close()
		s.conn = nil
	}
	return err
}

func (s *MulticastSender) MaxPacketSize() int {
	return s.mtu
}

func findInterfaceByIP(ip string) (*net.Interface, error) {
	e := errors.Template("findInterfaceByIP", errors.K.Invalid, "ip", ip)
	nip := net.ParseIP(ip)
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
			} else if ipaddr.Equal(nip) {
				return &iface, nil
			}
		}
	}
	return nil, e("reason", "interface not found")
}
