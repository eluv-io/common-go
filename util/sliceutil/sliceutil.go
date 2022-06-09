package sliceutil

import "reflect"

// Copy returns a shallow copy of the given slice. Returns nil if the given slice is nil. Returns an empty slice if
// the given slice is empty.
func Copy(target []interface{}) []interface{} {
	return CopyWithCap(target, 0)
}

// CopyWithCap returns a copy of the given slice. Returns nil if the given slice is nil. Returns an empty slice if
// the given slice is empty.
//
// The duplicate slice will be created with the given capacity (or the original capacity if smaller than the slice's
// length).
func CopyWithCap(target []interface{}, capacity int) []interface{} {
	if target == nil {
		return nil
	}

	c := cap(target)
	if capacity > c {
		c = capacity
	}

	dup := make([]interface{}, len(target), c)
	copy(dup, target)
	return dup
}

// Append appends all elements of source to target.
//
// If makeCopy is true, it returns the result in a newly allocated slice and leaves the source/target slices unchanged.
func Append(source []interface{}, target []interface{}, makeCopy bool) (res []interface{}) {
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
func Squash(source []interface{}, target []interface{}, makeCopy bool) (res []interface{}) {
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
func SquashAndDedupe(source []interface{}, target []interface{}, makeCopy bool) (res []interface{}) {
	if source == nil && target == nil {
		return nil
	}

	target = Dedupe(target, makeCopy)
	return Squash(source, target, makeCopy)
}

// Dedupe removes any duplicates of all elements in target slice.
//
// If makeCopy is true, it returns the result in a newly allocated slice and leaves the target slice unchanged.
func Dedupe(target []interface{}, makeCopy bool) (res []interface{}) {
	if target == nil {
		return nil
	}

	res = target[:0] // empty slice with same backing array as target
	if makeCopy {
		res = make([]interface{}, 0, len(target))
	}
	for _, el := range target {
		if !Contains(res, el) {
			res = append(res, el)
		}
	}
	return
}

// Contains returns true if the given slice contains the given elements, false otherwise.
func Contains(slice []interface{}, element interface{}) (res bool) {
	for _, el := range slice {
		if reflect.DeepEqual(el, element) {
			return true
		}
	}
	return false
}
