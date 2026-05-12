//go:build !linux

package io

import (
	"net"
)

func setPlatformOptions(conn *net.UDPConn) error {
	return nil
}
