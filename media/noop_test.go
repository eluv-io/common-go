package media

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNoopPacketizer_InitialState(t *testing.T) {
	p := NewNoopPacketizer()

	// next on a fresh packetizer should return (nil, nil).
	pkt, err := p.Next()
	require.NoError(t, err)
	require.Nil(t, pkt)

	// targetPacketSize should be zero before any packets are written.
	require.Zero(t, p.TargetPacketSize())
}

func TestNoopPacketizer_WriteThenNext(t *testing.T) {
	p := NewNoopPacketizer()

	input := []byte{0x01, 0x02, 0x03}

	p.Write(input)

	pkt, err := p.Next()
	require.NoError(t, err)

	// expect to get back the same slice that was written.
	require.NotNil(t, pkt)
	require.Len(t, pkt, len(input))
	require.Equal(t, input, pkt)

	// expect TargetPacketSize to match the last packet length.
	require.Equal(t, len(input), p.TargetPacketSize())

	// second Next() after the packet has been consumed should yield (nil, nil).
	pkt2, err := p.Next()
	require.NoError(t, err)
	require.Nil(t, pkt2)
}

func TestNoopPacketizer_MultipleWrites(t *testing.T) {
	p := NewNoopPacketizer()

	first := []byte{0xAA}
	second := []byte{0xBB, 0xCC}

	// first write
	p.Write(first)
	pkt1, err := p.Next()
	require.NoError(t, err)
	require.Len(t, pkt1, len(first))
	require.Equal(t, len(first), p.TargetPacketSize())

	// second write
	p.Write(second)
	pkt2, err := p.Next()
	require.NoError(t, err)
	require.Len(t, pkt2, len(second))
	require.Equal(t, len(second), p.TargetPacketSize())
}

func TestNoopPacketizer_ZeroLengthPacket(t *testing.T) {
	p := NewNoopPacketizer()

	empty := []byte{}

	p.Write(empty)
	pkt, err := p.Next()
	require.NoError(t, err)

	require.NotNil(t, pkt, "expected non-nil (but zero-length) packet")
	require.Len(t, pkt, 0)

	require.Equal(t, 0, p.TargetPacketSize())
}

func TestNoopPacer_NoPanics(t *testing.T) {

	p := NewNoopPacer()

	t.Run("Wait with nil slice", func(t *testing.T) {
		require.NotPanics(t, func() {
			p.Wait(nil)
		})
	})

	t.Run("Wait with non-nil slice", func(t *testing.T) {
		require.NotPanics(t, func() {
			p.Wait([]byte{0x01, 0x02})
		})
	})

	t.Run("SetDelay with arbitrary duration", func(t *testing.T) {
		require.NotPanics(t, func() {
			p.SetDelay(42 * time.Millisecond)
		})
	})
}

func TestNoopTransformer_Transform(t *testing.T) {

	tr := NewNoopTransformer()

	tests := []struct {
		name string
		in   []byte
	}{
		{"nil slice", nil},
		{"empty slice", []byte{}},
		{"non-empty slice", []byte{0x10, 0x20, 0x30}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out, err := tr.Transform(tc.in)
			require.NoError(t, err)

			if len(tc.in) == 0 {
				// For nil or empty, just ensure consistency of length and value.
				require.Len(t, out, len(tc.in))
				require.Equal(t, tc.in, out)
			} else {
				// Expect the exact same slice to be returned.
				require.Len(t, out, len(tc.in))
				require.Equal(t, tc.in, out)
				require.Equal(t, cap(tc.in), cap(out))
			}
		})
	}
}
