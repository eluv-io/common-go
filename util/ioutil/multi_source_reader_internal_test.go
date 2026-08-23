package ioutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMultiSourceReader_ChunkAheadOfOffsetIsInternalError exercises the read.off > r.off branch in Read directly,
// by injecting a chunk into r.reads that starts strictly ahead of r.off. No source added through the public Add
// API can ever produce this (see the comment on that branch), so this test lives in package ioutil (white-box)
// specifically to reach in and construct the otherwise-unreachable scenario, proving the defensive error path
// works correctly rather than hanging or panicking.
func TestMultiSourceReader_ChunkAheadOfOffsetIsInternalError(t *testing.T) {
	r := NewMultiSourceReader(nil)
	r.n.Add(1)
	defer func() { _ = r.Close() }()

	r.reads <- r.acquireRead([]byte("data"), 100, nil, nil)

	buf := make([]byte, 16)
	n, err := r.Read(buf)
	require.Equal(t, 0, n)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing bytes")
}
