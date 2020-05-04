package multiqueue

type set struct {
	head *entry
	tail *entry
	size int
}

func (s *set) Add(item *input) {
	e := &entry{val: item}
	if s.head == nil {
		s.head = e
		s.tail = e
	} else {
		s.tail.next = e
		e.prev = s.tail
		s.tail = e
	}
	s.size++
}

func (s *set) Remove(item *input) {
	e := s.head
	for e != nil {
		if e.val == item {
			s.remove(e)
			return
		}
	}
}

func (s *set) remove(e *entry) {
	if e == nil {
		return
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		s.tail = e.prev
	}
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		s.head = e.next
	}
	s.size--
}

func (s *set) Iterate(start *entry, fn func(e *entry) (cont bool)) {
	e := start
	size := s.size
	for i := 0; i < size; i++ {
		if e == nil {
			e = s.head
		}
		if !fn(e) {
			return
		}
		e = e.next
	}
}

type entry struct {
	next *entry
	prev *entry
	val  *input
}
