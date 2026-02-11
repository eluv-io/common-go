package io

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/eluv-io/common-go/util/httputil"
)

// LiveUrl represents a UDP live stream (unicast or multicast)
type LiveUrl struct {
	Scheme    string
	Host      string       // URL host (may be domain name or IP address)
	Addr      *net.UDPAddr // URL address (IP address)
	Group     *net.UDPAddr
	Multicast bool
	Port      int
	LocalAddr net.IP   // URL bind address if specified via query param 'localaddr'
	Sources   []net.IP // Optional multicast sources (query param 'sources') (not implemented currently)
	Reuse     bool     // Allow addr:port reuse (not implemented currently)
	TTL       int      // Optional TTL for sending mc packets (query param 'ttl')
	Loopback  bool     // Use loopback interface for sending packets
}

func ParseLiveUrlString(urlStr string) (*LiveUrl, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	return ParseLiveUrl(u)
}

// ParseLiveUrl parses live stream URLs into their components and resolves host and interfaces.
// Example:
// udp://host-100-10-10-1.contentfabric.io:11001
// udp://232.1.2.3:1234?localaddr=172.16.1.10&sources=10.0.0.5,10.0.0.6&reuse=1
func ParseLiveUrl(u *url.URL) (*LiveUrl, error) {
	out := &LiveUrl{
		Scheme: u.Scheme,
		TTL:    1,
	}

	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		return nil, fmt.Errorf("invalid host:port in URL (%s): %w", u.Host, err)
	}
	if strings.HasPrefix(host, "@") {
		return nil, fmt.Errorf("invalid host '%s': '@' prefix is not supported", host)
	}
	out.Host = host

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port %s: %w", portStr, err)
	}
	out.Port = port

	ip := net.ParseIP(host)
	if ip == nil {
		addrs, lookupErr := net.LookupIP(host)
		if lookupErr != nil {
			return nil, fmt.Errorf("unable to resolve host '%s': %w", host, lookupErr)
		}
		if len(addrs) == 0 {
			return nil, fmt.Errorf("unable to resolve host '%s': no addresses returned", host)
		}
		for _, candidate := range addrs {
			if candidate.To4() != nil {
				ip = candidate
				break
			}
		}
		if ip == nil {
			ip = addrs[0]
		}
	}
	if ip == nil {
		return nil, fmt.Errorf("unable to determine IP for host '%s'", host)
	}
	out.Addr = &net.UDPAddr{IP: ip, Port: out.Port}
	if ip.IsMulticast() {
		out.Multicast = true
		out.Group = &net.UDPAddr{IP: ip, Port: out.Port}
	}

	// Parse query parameters
	q := u.Query()

	if la := q.Get("localaddr"); la != "" {
		laIp := net.ParseIP(la)
		if laIp == nil {
			return nil, fmt.Errorf("localaddr is not a valid IP: %s", la)
		}
		out.LocalAddr = laIp
	}

	if srcs := q.Get("sources"); srcs != "" {
		for _, s := range strings.Split(srcs, ",") {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			ip := net.ParseIP(s)
			if ip == nil {
				return nil, fmt.Errorf("invalid source IP: %s", s)
			}
			out.Sources = append(out.Sources, ip)
		}
	}

	out.Reuse = httputil.BoolQuery(q, "reuse", false)

	out.TTL = httputil.IntQuery(q, "ttl", 1)
	if out.TTL < 1 {
		return nil, fmt.Errorf("invalid ttl: %d", out.TTL)
	}
	out.Loopback = httputil.BoolQuery(q, "loopback", false)

	return out, nil
}

func interfaceByIP(ip net.IP) (*net.Interface, error) {
	if ip == nil {
		return nil, nil
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("unable to list interfaces: %w", err)
	}
	for i := range ifaces {
		iface := &ifaces[i]
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ifaceIP net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ifaceIP = v.IP
			case *net.IPAddr:
				ifaceIP = v.IP
			}
			if ifaceIP != nil && ifaceIP.Equal(ip) {
				return iface, nil
			}
		}
	}
	return nil, fmt.Errorf("no interface found with IP %s", ip)
}
