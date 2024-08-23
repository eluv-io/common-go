package structured

import "github.com/eluv-io/errors-go"

// MergeOptions are the options available for merge operations.
type MergeOptions struct {
	// MakeCopy controls whether target/source structures are modified in place or merged in to a separate structure.
	//
	// If MakeCopy is false, the target and/or source structures may be modified.
	//
	// If MakeCopy is true, the target and source structures remain unmodified. Instead, the merged data is copied
	// into a separate structure. Note, however, that the copy is shallow, so the result might still refer back to maps,
	// slices or other objects in the target or source structures. Modifying those subsequently will therefore modify
	// target or sources.
	MakeCopy bool

	// The mode for merging arrays. See ArrayMergeMode.
	ArrayMergeMode ArrayMergeMode
}

// =====================================================================================================================

// ArrayMergeMode defines a mode for merging arrays
type ArrayMergeMode string

func (m ArrayMergeMode) Validate() error {
	switch m {
	case "",
		ArrayMergeModes.Append(),
		ArrayMergeModes.Squash(),
		ArrayMergeModes.Dedupe(),
		ArrayMergeModes.Replace():
		return nil
	}
	return errors.NoTrace("ArrayMergeMode.Validate", errors.K.Invalid, "mode", m)
}

// ArrayMergeModes is the enum of ArrayMergeMode.
const ArrayMergeModes arrayMergeModeEnum = 0

type arrayMergeModeEnum int

// Append mode appends all elements of the source array to the end of the target array. Duplicates are not removed.
func (arrayMergeModeEnum) Append() ArrayMergeMode { return "append" }

// Squash mode appends all elements of the source array to the end of the target array, except if the same element
// already exists in the merged array (right before the element is appended).
//
// In other words:
//   - source elements that exist in the target are not appended
//   - duplicate elements in the source are appended at most once
//   - duplicate elements in the target array remain
func (arrayMergeModeEnum) Squash() ArrayMergeMode { return "squash" }

// Dedupe mode appends all elements of the source array to the end of the target array and then removes any duplicates.
//
// In other words:
//   - source elements that exist in the target are not appended
//   - duplicate elements in the source are appended at most once
//   - duplicate elements in the target array are removed
func (arrayMergeModeEnum) Dedupe() ArrayMergeMode { return "dedupe" }

// Replace mode replaces the target array with the source array. No merging occurs.
func (arrayMergeModeEnum) Replace() ArrayMergeMode { return "replace" }
