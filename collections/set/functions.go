package set

import (
	"sort"

	"golang.org/x/exp/constraints"
)

// CompareFn is a comparison function that returns
//   - 0 if e1 == e2
//   - -1 if e1 < e2
//   - +1 if e1 > e2
type CompareFn[T any] func(e1, e2 T) int

// Compare compares two values of an ordered type, returning
//   - 0 if e1 == e2
//   - -1 if e1 < e2
//   - +1 if e1 > e2
func Compare[T constraints.Ordered](e1, e2 T) int {
	if e1 == e2 {
		return 0
	}
	if e1 < e2 {
		return -1
	}
	return +1
}

// Contains return true if the ordered set contains the given element, false otherwise.
func Contains[T constraints.Ordered](set []T, elem T) bool {
	return ContainsFn(Compare[T], set, elem)
}

// ContainsFn return true if the ordered set contains the given element, false otherwise.
func ContainsFn[T any](cmp CompareFn[T], set []T, elem T) bool {
	_, found := sort.Find(len(set), func(i int) int {
		return cmp(elem, set[i])
	})
	return found
}

// Insert inserts elements into an ordered set, and returns the new ordered set. If an element is already contained
// in the set, the set remains unchanged.
func Insert[T constraints.Ordered](set []T, elements ...T) []T {
	return InsertFn(Compare[T], set, elements...)
}

// InsertFn inserts elements into an ordered set, and returns the new ordered set. If an element is already contained
// in the set, the set remains unchanged.
func InsertFn[T any](cmp CompareFn[T], set []T, elements ...T) []T {
	// grow the slice if needed
	if len(set)+len(elements) > cap(set) {
		nset := make([]T, len(set), len(set)+len(elements))
		copy(nset, set)
		set = nset
	}

	for _, elem := range elements {
		idx, found := sort.Find(len(set), func(i int) int {
			return cmp(elem, set[i])
		})
		if found {
			continue
		}
		// make room for new element
		set = append(set, elem)
		// move more recent elements away from insertion index
		copy(set[idx+1:], set[idx:])
		// insert the new element
		set[idx] = elem
	}
	return set
}

// Remove removes elements from an ordered set, and returns the new ordered set. If an element is not in the set, the
// set remains unchanged.
func Remove[T constraints.Ordered](set []T, elements ...T) []T {
	return RemoveFn(Compare[T], set, elements...)
}

// RemoveFn removes elements from an ordered set, and returns the new ordered set. If an element is not in the set, the
// set remains unchanged.
func RemoveFn[T any](cmp CompareFn[T], set []T, elements ...T) []T {
	for _, elem := range elements {
		idx, found := sort.Find(len(set), func(i int) int {
			return cmp(elem, set[i])
		})
		if !found {
			continue
		}
		// move larger elements one up
		copy(set[idx:], set[idx+1:])
		// remove last element
		var zero T
		set[len(set)-1] = zero
		set = set[:len(set)-1]
	}
	return set
}
