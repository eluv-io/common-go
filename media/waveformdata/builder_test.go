package waveformdata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuilderSequential(t *testing.T) {
	b := NewBuilder(1, 48000, 2400)
	require.Equal(t, int64(-1), b.FirstBucket())

	require.NoError(t, b.Add(0, []int16{-1, 1, -2, 2}))
	require.NoError(t, b.Add(2, []int16{-3, 3}))
	require.Equal(t, int64(0), b.FirstBucket())
	require.Equal(t, int64(3), b.Length())
	require.Equal(t, int64(0), b.GapBuckets())

	w := b.Build()
	require.Equal(t, []int16{-1, 1, -2, 2, -3, 3}, w.Data)
	require.Equal(t, 2400, w.SamplesPerPixel)
}

func TestBuilderMergesPartialBuckets(t *testing.T) {
	// Part one ends with a partial bucket at index 2 covering only quiet samples, part two starts in the same bucket
	// with the loud samples. The merged bucket must be the full min/max.
	b := NewBuilder(2, 48000, 2400)
	require.NoError(t, b.Add(0, []int16{
		-1, 1, -1, 1,
		-2, 2, -2, 2,
		-3, 3, 0, 0, // partial tail of part one
	}))
	require.NoError(t, b.Add(2, []int16{
		-30, 30, -5, 5, // partial head of part two, same bucket index
		-4, 4, -4, 4,
	}))
	require.Equal(t, int64(4), b.Length())
	require.Equal(t, []int16{
		-1, 1, -1, 1,
		-2, 2, -2, 2,
		-30, 30, -5, 5,
		-4, 4, -4, 4,
	}, b.Build().Data)
}

func TestBuilderGapsAndOffsets(t *testing.T) {
	// A range request starting at bucket 100: the builder's first bucket is the first index added.
	b := NewBuilder(1, 48000, 2400)
	require.NoError(t, b.Add(100, []int16{-1, 1}))
	require.NoError(t, b.Add(103, []int16{-4, 4}))
	require.Equal(t, int64(100), b.FirstBucket())
	require.Equal(t, int64(4), b.Length())
	require.Equal(t, int64(2), b.GapBuckets())
	require.Equal(t, []int16{-1, 1, 0, 0, 0, 0, -4, 4}, b.Build().Data)
}

func TestBuilderRejects(t *testing.T) {
	b := NewBuilder(2, 48000, 2400)
	require.Error(t, b.Add(0, []int16{1, 2, 3}), "not whole buckets")
	require.NoError(t, b.Add(5, []int16{-1, 1, -1, 1}))
	require.Error(t, b.Add(4, []int16{-1, 1, -1, 1}), "precedes the builder start")

	empty := NewBuilder(1, 48000, 2400).Build()
	require.NotNil(t, empty.Data)
	require.Equal(t, 0, empty.Length())
}
