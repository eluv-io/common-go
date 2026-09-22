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

	kept, idrFlag, changed := inspectAccessUnit([][]byte{aud, sps, pps, idr, filler}, [][]byte{sps}, [][]byte{pps})
	require.Equal(t, [][]byte{sps, pps, idr}, kept, "AUD and filler dropped, parameter sets kept")
	require.True(t, idrFlag)
	require.False(t, changed)

	_, idrFlag, changed = inspectAccessUnit([][]byte{nonIdr}, [][]byte{sps}, [][]byte{pps})
	require.False(t, idrFlag)
	require.False(t, changed)

	changedSps := []byte{0x67, 1, 2, 4}
	_, _, changed = inspectAccessUnit([][]byte{changedSps, pps, idr}, [][]byte{sps}, [][]byte{pps})
	require.True(t, changed)
}

// TestInspectAccessUnit_CompactsInPlace pins the aliasing the fragment arena relies on: the kept NAL units are the
// prefix of the slice handed in, so a sample's views need no slice of their own.
func TestInspectAccessUnit_CompactsInPlace(t *testing.T) {
	sps := []byte{0x67, 1}
	idr := []byte{0x65, 2}
	aud := []byte{0x09, 0xf0}

	in := [][]byte{aud, sps, aud, idr}
	kept, _, _ := inspectAccessUnit(in, [][]byte{sps}, nil)
	require.Len(t, kept, 2)
	require.Equal(t, &in[0][0], &kept[0][0], "the kept units are moved to the front of the input slice")
	require.Equal(t, [][]byte{sps, idr}, kept)
}

func TestAppendNALUs(t *testing.T) {
	idr := []byte{0x65, 5, 6}
	sps := []byte{0x67, 1}

	dst := make([][]byte, 0, 4)
	dst, err := appendNALUs(dst, avcc(sps, idr), 4)
	require.NoError(t, err)
	require.Equal(t, [][]byte{sps, idr}, dst)

	// a second sample appends to the same arena
	dst, err = appendNALUs(dst, avcc(idr), 4)
	require.NoError(t, err)
	require.Len(t, dst, 3)

	// a failed split leaves the arena as it was
	before := len(dst)
	dst, err = appendNALUs(dst, []byte{0, 0, 0, 9, 1}, 4)
	require.Error(t, err)
	require.Len(t, dst, before)
}
