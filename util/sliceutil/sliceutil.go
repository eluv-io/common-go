package sliceutil

import (
	"reflect"

	"golang.org/x/exp/constraints"
)

// Clone is an alias of Copy and returns a shallow copy of the given slice. Returns nil if the given slice is nil.
// Returns an empty slice if the given slice is empty.
func Clone[T any](target []T) []T {
	return CopyWithCap(target, 0)
}

// Copy returns a shallow copy of the given slice. Returns nil if the given slice is nil. Returns an empty slice if
// the given slice is empty.
func Copy[T any](target []T) []T {
	return CopyWithCap(target, 0)
}

// CopyWithCap returns a copy of the given slice. Returns nil if the given slice is nil. Returns an empty slice if
// the given slice is empty.
//
// The duplicate slice will be created with the given capacity (or the original capacity if smaller than the slice's
// length).
func CopyWithCap[T any](target []T, capacity int) []T {
	if target == nil {
		return nil
	}

	c := cap(target)
	if capacity > c {
		c = capacity
	}

	dup := make([]T, len(target), c)
	copy(dup, target)
	return dup
}

// Append appends all elements of source to target.
//
// If makeCopy is true, it returns the result in a newly allocated slice and leaves the source/target slices unchanged.
func Append[T any](source []T, target []T, makeCopy bool) (res []T) {
	if source == nil && target == nil {
		return nil
	}

	res = target
	if makeCopy {
		res = CopyWithCap(target, len(target)+len(source))
	}
	res = append(res, source...)
	return
}

// Squash appends all elements of source to target, unless an element exists already in the target or any of the already
// appended elements.
//
// If makeCopy is true, it returns the result in a newly allocated slice and leaves the source/target slices unchanged.
func Squash[T any](source []T, target []T, makeCopy bool) (res []T) {
	if source == nil && target == nil {
		return nil
	}

	res = target
	if makeCopy {
		res = CopyWithCap(target, len(target)+len(source))
	}

	for _, el := range source {
		if !Contains(res, el) {
			res = append(res, el)
		}
	}
	return
}

// SquashAndDedupe appends all elements of source to target and returns the deduped result.
//
// If makeCopy is true, it returns the result in a newly allocated slice and leaves the source/target slices unchanged.
func SquashAndDedupe[T any](source []T, target []T, makeCopy bool) (res []T) {
	if source == nil && target == nil {
		return nil
	}

	target = Dedupe(target, makeCopy)
	return Squash(source, target, makeCopy)
}

// Dedupe removes any duplicates of all elements in target slice.
//
// If makeCopy is true, it returns the result in a newly allocated slice and leaves the target slice unchanged.
func Dedupe[T any](target []T, makeCopy bool) (res []T) {
	if target == nil {
		return nil
	}

	res = target[:0] // empty slice with same backing array as target
	if makeCopy {
		res = make([]T, 0, len(target))
	}
	for _, el := range target {
		if !Contains(res, el) {
			res = append(res, el)
		}
	}
	return
}

// Equaler is the interface for structs implementing an Equal() function.
type Equaler[T any] interface {
	Equal(other T) bool
}

func eq[T any](e1, e2 T) bool {
	return any(e1).(Equaler[T]).Equal(e2)
}

func deepEqual[T any](e1, e2 T) bool {
	return reflect.DeepEqual(e1, e2)
}

// Contains returns true if the given slice contains the given elements, false otherwise.
func Contains[T any](slice []T, element T) (res bool) {
	if _, ok := any(element).(Equaler[T]); ok {
		return ContainsFn(slice, element, eq[T])
	}
	return ContainsFn(slice, element, deepEqual[T])
}

// ContainsFn returns true if the slice contains the given element using the provided function to compare elements,
// false otherwise.
func ContainsFn[T any](slice []T, element T, equal func(e1, e2 T) bool) (res bool) {
	for _, el := range slice {
		if equal(el, element) {
			return true
		}
	}
	return false
}

// Remove removes all occurrences of an element from the given slice. Returns the new slice and the number of removed
// elements.
func Remove[T any](slice []T, element T) ([]T, int) {
	if _, ok := any(element).(Equaler[T]); ok {
		return RemoveFn(slice, element, eq[T])
	}
	return RemoveFn(slice, element, func(e1, e2 T) bool {
		return reflect.DeepEqual(e1, e2)
	})
}

// RemoveFn removes all occurrences of an element from the given slice, using the provided function to compare elements.
// Returns the new slice and the number of removed elements.
func RemoveFn[T any](slice []T, element T, equal func(e1, e2 T) bool) ([]T, int) {
	return RemoveMatch(slice, func(e T) bool {
		return equal(e, element)
	})
}

// RemoveMatch removes all elements that match according to the provided match function from the given slice. Removal is
// performed inline, freed up slots at the end of the slice are zeroed out. Returns the updated slice and the number of
// removed elements.
func RemoveMatch[T any](slice []T, match func(e T) bool) ([]T, int) {
	var zero T
	removed := 0
	for i := 0; i < len(slice); i++ {
		if match(slice[i]) {
			removed++
			slice[i] = zero
		} else {
			if removed > 0 {
				slice[i-removed] = slice[i]
				slice[i] = zero
			}
		}
	}
	return slice[:len(slice)-removed], removed
}

// RemoveIndex removes the element at the given index from the provided slice. Removes nothing if the index is
// out-of-bounds.
func RemoveIndex[T any](slice []T, idx int) []T {
	if idx < 0 || idx >= len(slice) {
		return slice
	}
	var zero T
	copy(slice[idx:], slice[idx+1:])
	slice[len(slice)-1] = zero
	return slice[:len(slice)-1]
}

// Compare compares the elements of s1 and s2. The elements are compared sequentially, starting at index 0, until one
// element is not equal to the other. The result of comparing the first non-matching elements is returned. If both
// slices are equal until one of them ends, the shorter slice is considered less than the longer one. The result is 0 if
// s1 == s2, -1 if s1 < s2, and +1 if s1 > s2. Comparisons involving floating point NaNs are ignored.
func Compare[T constraints.Ordered](s1, s2 []T) int {
	s2len := len(s2)
	for i, v1 := range s1 {
		if i >= s2len {
			return +1
		}
		v2 := s2[i]
		switch {
		case v1 < v2:
			return -1
		case v1 > v2:
			return +1
		}
	}
	if len(s1) < s2len {
		return -1
	}
	return 0
}

// Reverse reverses the order of the elements in the provided slice.
func Reverse[T any](slice []T) {
	max := len(slice) - 1
	for i := 0; i < (max+1)/2; i++ {
		slice[i], slice[max-i] = slice[max-i], slice[i]
	}
}

// First returns the first element of the given slice. Returns the zero value if the slice is empty.
func First[T any](slice []T) T {
	if len(slice) == 0 {
		var zero T
		return zero
	}
	return slice[0]
}

// Last returns the last element of the given slice. Returns the zero value if the slice is empty.
func Last[T any](slice []T) T {
	if len(slice) == 0 {
		var zero T
		return zero
	}
	return slice[len(slice)-1]
}
