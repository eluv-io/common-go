package samplepacer

// itemHeap is a binary min-heap of items ordered by DTS, video before audio at equal times.
//
// It replaces container/heap, whose Interface takes and returns `any`: with items held by value, every push and pop
// through that interface boxes the item onto the heap, which on this path is one allocation per sample in each
// direction. The operations below are the textbook ones, specialised to the element type.
type itemHeap[P any] []Item[P]

func (h itemHeap[P]) less(i, j int) bool {
	if h[i].DTS != h[j].DTS {
		return h[i].DTS < h[j].DTS
	}
	return h[i].Track < h[j].Track
}

func (h *itemHeap[P]) push(it Item[P]) {
	*h = append(*h, it)
	h.up(len(*h) - 1)
}

// pop removes and returns the smallest item. It zeroes the vacated slot so the backing array does not pin the
// payload of an item that has left the queue.
func (h *itemHeap[P]) pop() Item[P] {
	old := *h
	n := len(old) - 1
	it := old[0]
	old[0] = old[n]
	var zero Item[P]
	old[n] = zero
	*h = old[:n]
	if n > 0 {
		h.down(0)
	}
	return it
}

func (h itemHeap[P]) up(i int) {
	for i > 0 {
		parent := (i - 1) / 2
		if !h.less(i, parent) {
			break
		}
		h[i], h[parent] = h[parent], h[i]
		i = parent
	}
}

func (h itemHeap[P]) down(i int) {
	n := len(h)
	for {
		left := 2*i + 1
		if left >= n {
			break
		}
		smallest := left
		if right := left + 1; right < n && h.less(right, left) {
			smallest = right
		}
		if !h.less(smallest, i) {
			break
		}
		h[i], h[smallest] = h[smallest], h[i]
		i = smallest
	}
}
