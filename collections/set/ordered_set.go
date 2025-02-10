package set

import (
	"encoding/json"
	"fmt"

	"github.com/fxamacker/cbor/v2"
	"golang.org/x/exp/constraints"

	"github.com/eluv-io/common-go/util/sliceutil"
)

// NewOrderedSet creates a new ordered set. Elements have to implement constraints.Ordered. If not, use
// NewOrderedSetFn().
func NewOrderedSet[T constraints.Ordered](elements ...T) *OrderedSet[T] {
	return NewOrderedSetFn(Compare[T], elements...)
}

// NewOrderedSetFn creates a nwe ordered set, using the given function to compare elements.
func NewOrderedSetFn[T any](compare func(e1, e2 T) int, elements ...T) *OrderedSet[T] {
	var set = OrderedSet[T]{
		compare: compare,
	}
	set.Insert(elements...)
	return &set
}

// An OrderedSet is a collection that a) is ordered and b) has no duplicate entries. It should be used with a small
// number of elements only, since it is implemented as a simple slice.
//
// Use one of the constructor functions NewOrderedSet or NewOrderedSetFn to create an instance in order to ensure proper
// initialization.
type OrderedSet[T any] struct {
	set     []T
	compare func(e1, e2 T) int
}

// Clone returns a shallow copy of this set.
func (s *OrderedSet[T]) Clone() *OrderedSet[T] {
	res := *s
	res.set = sliceutil.Clone(s.set)
	return &res
}

// String returns a string representation of this set in the standard go slice format.
func (s *OrderedSet[T]) String() string {
	return fmt.Sprint(s.set)
}

// Elements returns the elements of the set as a slice.
func (s *OrderedSet[T]) Elements() []T {
	cp := make([]T, len(s.set))
	copy(cp, s.set)
	return cp
}

// Insert inserts elements into the ordered set. If an element is already contained in the set, the set remains
// unchanged.
func (s *OrderedSet[T]) Insert(elements ...T) {
	s.set = InsertFn(s.compare, s.set, elements...)
}

// Remove removes elements from the ordered set. If an element is not in the set, the set remains unchanged.
func (s *OrderedSet[T]) Remove(elements ...T) {
	s.set = RemoveFn(s.compare, s.set, elements...)
}

// Contains return true if the ordered set contains the given element, false otherwise.
func (s *OrderedSet[T]) Contains(elem T) bool {
	return ContainsFn(s.compare, s.set, elem)
}

// Size returns the number of elements in the set.
func (s *OrderedSet[T]) Size() int {
	return len(s.set)
}

// Clear removes all elements from the set.
func (s *OrderedSet[T]) Clear() {
	s.set = nil
}

// MarshalJSON marshals this set as a JSON array.
func (s *OrderedSet[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.set)
}

// UnmarshalJSON unmarshals this set from a JSON array.
func (s *OrderedSet[T]) UnmarshalJSON(bts []byte) error {
	var slice []T
	err := json.Unmarshal(bts, &slice)
	if err != nil {
		return err
	}
	s.Insert(slice...)
	return nil
}

// MarshalCBOR marshals this set as a CBOR array.
func (s *OrderedSet[T]) MarshalCBOR() ([]byte, error) {
	return cbor.Marshal(s.Elements())
}

// UnmarshalCBOR unmarshals this set from a CBOR array.
func (s *OrderedSet[T]) UnmarshalCBOR(bts []byte) error {
	var slice []T
	err := cbor.Unmarshal(bts, &slice)
	if err != nil {
		return err
	}
	s.Insert(slice...)
	return nil
}
