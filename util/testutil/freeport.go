package testutil

import (
	"fmt"
	"net"
)

// PortListener returns a net.Listener on the given port on 127.0.0.1
// If port is 0, a random free port will be used.
func PortListener(targetPort int) (listener net.Listener, actualPort int, err error) {
	address := fmt.Sprintf("127.0.0.1:%d", targetPort)
	listener, err = net.Listen("tcp", address)
	if err != nil {
		return nil, 0, err
	}

	return listener, listener.Addr().(*net.TCPAddr).Port, nil
}

// FreePortListener returns a net.Listener on any free port on 127.0.0.1
func FreePortListener() (net.Listener, int, error) {
	return PortListener(0)
}

// FreePort returns a free IP (UDP/TCP) port
func FreePort() (int, error) {
	listener, port, err := PortListener(0)
	if err == nil {
		err = listener.Close()
	}
	if err != nil {
		return 0, err
	}
	return port, nil
}
