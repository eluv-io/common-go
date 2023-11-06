package sliceutil_test

import (
	"fmt"

	"github.com/eluv-io/common-go/util/sliceutil"
)

func ExampleRemoveMatch() {
	slice := []int{1, 2, 3, 1, 2, 3, 6, 6, 4, 2, 4}
	res, i := sliceutil.RemoveMatch(slice, func(e int) bool {
		return e >= 3
	})
	fmt.Printf("removed %d elements: %v\n", i, res)
	fmt.Printf("removed slots in original slice are zeroed: %v\n", slice)

	// Output:
	//
	// removed 6 elements: [1 2 1 2 2]
	// removed slots in original slice are zeroed: [1 2 1 2 2 0 0 0 0 0 0]
}
