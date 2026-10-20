package fmp4

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func avcc(nalus ...[]byte) []byte {
	var out []byte
	for _, n := range nalus {
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(n)))
		out = append(out, l[:]...)
		out = append(out, n...)
	}
	return out
}

func TestSplitAVCC(t *testing.T) {
	sps := []byte{0x67, 1, 2, 3}
	pps := []byte{0x68, 4}
	idr := []byte{0x65, 5, 6, 7, 8, 9}
	aud := []byte{0x09, 0xf0}

	nalus, err := SplitAVCC(avcc(aud, sps, pps, idr), 4)
	require.NoError(t, err)
	require.Equal(t, [][]byte{aud, sps, pps, idr}, nalus)

	t.Run("two byte prefixes", func(t *testing.T) {
		sample := []byte{0, 2, 0x65, 1, 0, 1, 0x09}
		nalus, err := SplitAVCC(sample, 2)
		require.NoError(t, err)
		require.Equal(t, [][]byte{{0x65, 1}, {0x09}}, nalus)
	})

	t.Run("errors", func(t *testing.T) {
		_, err := SplitAVCC(avcc(idr)[:5], 4)
		require.ErrorContains(t, err, "truncated NALU")
		_, err = SplitAVCC([]byte{0, 0}, 4)
		require.ErrorContains(t, err, "truncated NALU length prefix")
		_, err = SplitAVCC([]byte{0, 0, 0, 0}, 4)
		require.ErrorContains(t, err, "zero-length NALU")
		_, err = SplitAVCC(avcc(idr), 5)
		require.ErrorContains(t, err, "unsupported NALU length size")
	})
}

func TestInspectAccessUnit(t *testing.T) {
	sps := []byte{0x67, 1, 2, 3}
	pps := []byte{0x68, 4}
	idr := []byte{0x65, 5, 6}
	nonIdr := []byte{0x41, 7}
	aud := []byte{0x09, 0xf0}
	filler := []byte{0x0c, 0xff}

	au := inspectAccessUnit([][]byte{aud, sps, pps, idr, filler}, [][]byte{sps}, [][]byte{pps})
	require.Equal(t, [][]byte{sps, pps, idr}, au.nalus, "AUD and filler dropped, parameter sets kept")
	require.True(t, au.idr)
	require.False(t, au.paramSetChange)

	au = inspectAccessUnit([][]byte{nonIdr}, [][]byte{sps}, [][]byte{pps})
	require.False(t, au.idr)
	require.False(t, au.paramSetChange)

	changedSps := []byte{0x67, 1, 2, 4}
	au = inspectAccessUnit([][]byte{changedSps, pps, idr}, [][]byte{sps}, [][]byte{pps})
	require.True(t, au.paramSetChange)
}
