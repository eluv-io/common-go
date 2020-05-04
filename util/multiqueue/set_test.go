package multiqueue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSet(t *testing.T) {
	in := make([]*input, 10)
	for i := 0; i < 10; i++ {
		in[i] = &input{cap: i}
	}

	s := set{}

	for i := 0; i < 10; i++ {
		s.Add(in[i])
		require.Equal(t, i+1, s.size)
	}

	iter := make([]*input, 0)
	s.Iterate(nil, func(e *entry) (cont bool) {
		iter = append(iter, e.val)
		return true
	})
	require.Equal(t, in, iter)

	iter = make([]*input, 0)
	i := 0
	s.Iterate(nil, func(e *entry) (cont bool) {
		iter = append(iter, e.val)
		if i >= 7 {
			s.remove(e)
		}
		i++
		return true
	})

	require.Equal(t, 10, len(iter))
	require.Equal(t, 7, s.size)
}
