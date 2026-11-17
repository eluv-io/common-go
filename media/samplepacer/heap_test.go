package samplepacer

import (
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestItemHeap_OrdersLikeSort checks the hand-rolled heap against a sort of the same items: by decode time, video
// before audio at equal times.
func TestItemHeap_OrdersLikeSort(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	for _, n := range []int{0, 1, 2, 7, 64, 500} {
		want := make([]Item[int], n)
		for i := range want {
			want[i] = Item[int]{
				Track:   Track(rnd.Intn(2)),
				DTS:     time.Duration(rnd.Intn(50)) * 10 * time.Millisecond,
				Payload: i,
			}
		}
		var h itemHeap[int]
		for _, it := range want {
			h.push(it)
		}
		require.Len(t, h, n)

		sort.SliceStable(want, func(i, j int) bool {
			if want[i].DTS != want[j].DTS {
				return want[i].DTS < want[j].DTS
			}
			return want[i].Track < want[j].Track
		})
		for i := 0; i < n; i++ {
			got := h.pop()
			require.Equal(t, want[i].DTS, got.DTS, "item %d of %d", i, n)
			require.Equal(t, want[i].Track, got.Track, "item %d of %d", i, n)
		}
		require.Empty(t, h)
	}
}

// TestItemHeap_PopClearsTheSlot pins that a popped item's payload is not left in the backing array, where it would
// keep a pooled payload alive after it was handed on.
func TestItemHeap_PopClearsTheSlot(t *testing.T) {
	var h itemHeap[*int]
	v := 7
	h.push(Item[*int]{DTS: time.Second, Payload: &v})
	h.push(Item[*int]{DTS: 2 * time.Second, Payload: &v})
	h.pop()
	h.pop()
	require.Empty(t, h)
	full := h[:cap(h)]
	for i := range full {
		require.Nil(t, full[i].Payload, "slot %d still holds a payload", i)
	}
}
