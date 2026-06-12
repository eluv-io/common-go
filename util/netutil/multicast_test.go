package netutil

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/byteutil"
)

func TestMulticast(t *testing.T) {
	port := "8888"
	multicastAddr := net.JoinHostPort("239.0.0.123", port)
	interfaceAddr := net.JoinHostPort("127.0.0.1", port)

	for _, ctx := range []context.Context{nil, context.Background()} {
		recv, err := NewMulticastReceiver(ctx, multicastAddr, interfaceAddr)
		require.NoError(t, err)

		// Unicast
		send, err := NewMulticastSender(ctx, interfaceAddr, interfaceAddr)
		require.NoError(t, err)
		data := byteutil.RandomBytes(MaxSendPacketSize + 1)
		err = send.WritePacket(data)
		require.Error(t, err)
		for i := range 10 {
			data := byteutil.RandomBytes(MaxSendPacketSize)
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
		// send, err = NewMulticastSender(ctx, multicastAddr, interfaceAddr)
		// require.NoError(t, err)
		// data = byteutil.RandomBytes(MaxSendPacketSize + 1)
		// err = send.WritePacket(data)
		// require.Error(t, err)
		// for i := range 10 {
		// 	data := byteutil.RandomBytes(MaxSendPacketSize)
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
}
