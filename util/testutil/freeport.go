package testutil

import (
	"fmt"
	"net"
)

func PortListener(port int) (net.Listener, int, error) {
	address := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, 0, err
	}

	return listener, listener.Addr().(*net.TCPAddr).Port, nil
}

func FreePortListener() (net.Listener, int, error) {
	return PortListener(0)
}
