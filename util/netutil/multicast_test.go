package netutil

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/byteutil"
)

func TestMulticast(t *testing.T) {
	port := "8888"
	multicastIP := "239.0.0.123"
	localIP := "127.0.0.1"

	recv, err := NewMulticastReceiver(net.JoinHostPort(multicastIP, port), localIP)
	require.NoError(t, err)

	// Unicast
	size := 1024
	send, err := NewMulticastSender(net.JoinHostPort(localIP, port), size)
	require.NoError(t, err)
	mtu := send.MaxPacketSize()
	require.Equal(t, size, mtu)
	data := byteutil.RandomBytes(mtu + 1)
	err = send.WritePacket(data)
	require.Error(t, err)
	for i := range 10 {
		data := byteutil.RandomBytes(mtu)
		err := send.WritePacket(data)
		require.NoError(t, err, i)
		packet, isMulticast, source, err := recv.ReadPacket()
		require.NoError(t, err, i)
		require.Equal(t, data, packet, i)
		require.False(t, isMulticast, i)
		require.NotEmpty(t, source, i)
	}
	err = send.Close()
	require.NoError(t, err)

	// DISABLED - depends on network configuration of host
	// // Multicast
	// send, err = NewMulticastSender(net.JoinHostPort(multicastIP, port))
	// require.NoError(t, err)
	// mtu = send.MaxPacketSize()
	// data = byteutil.RandomBytes(mtu+1)
	// err = send.WritePacket(data)
	// require.Error(t, err)
	// for i := range 10 {
	// 	data := byteutil.RandomBytes(mtu)
	// 	err := send.WritePacket(data)
	// 	require.NoError(t, err, i)
	// 	packet, isMulticast, source, err := recv.ReadPacket()
	// 	require.NoError(t, err, i)
	// 	require.Equal(t, data, packet, i)
	// 	require.True(t, isMulticast, i)
	// 	require.NotEmpty(t, source, i)
	// }
	// err = send.Close()
	// require.NoError(t, err)

	err = recv.Close()
	require.NoError(t, err)
}
