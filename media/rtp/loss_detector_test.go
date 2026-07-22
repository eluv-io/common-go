package rtp

import (
	"testing"

	pionrtp "github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestLossDetectorCompatibilityMethods(t *testing.T) {
	t.Run("packet", func(t *testing.T) {
		var detector LossDetector
		require.NoError(t, detector.Next(&pionrtp.Packet{Header: pionrtp.Header{SequenceNumber: 10}}))
		require.NoError(t, detector.Next(&pionrtp.Packet{Header: pionrtp.Header{SequenceNumber: 11}}))
		require.Error(t, detector.Next(&pionrtp.Packet{Header: pionrtp.Header{SequenceNumber: 13}}))
	})

	t.Run("packet alias", func(t *testing.T) {
		var detector LossDetector
		require.NoError(t, detector.NextPacket(&pionrtp.Packet{Header: pionrtp.Header{SequenceNumber: 10}}))
		require.NoError(t, detector.NextPacket(&pionrtp.Packet{Header: pionrtp.Header{SequenceNumber: 11}}))
	})

	t.Run("sequence", func(t *testing.T) {
		var detector LossDetector
		require.NoError(t, detector.NextSequence(0xffff))
		require.NoError(t, detector.NextSequence(0))
	})
}
