package sliceutil_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/sliceutil"
)

// baseline current version:
//
//	goos: darwin
//	goarch: amd64
//	pkg: github.com/eluv-io/common-go/util/sliceutil
//	cpu: Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz
//	BenchmarkRemoveMatch
//	BenchmarkRemoveMatch/slice_len_10
//	BenchmarkRemoveMatch/slice_len_10-16         	  487131	      2367 ns/op
//	BenchmarkRemoveMatch/slice_len_100
//	BenchmarkRemoveMatch/slice_len_100-16        	  387724	      2637 ns/op
//	BenchmarkRemoveMatch/slice_len_1000
//	BenchmarkRemoveMatch/slice_len_1000-16       	  232868	      5655 ns/op
//	BenchmarkRemoveMatch/slice_len_10000
//	BenchmarkRemoveMatch/slice_len_10000-16      	   40525	     30455 ns/op
//	BenchmarkRemoveMatch/slice_len_100000
//	BenchmarkRemoveMatch/slice_len_100000-16     	    4180	    307099 ns/op
//	PASS
//
// original version:
//
//	goos: darwin
//	goarch: amd64
//	pkg: github.com/eluv-io/common-go/util/sliceutil
//	cpu: Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz
//	BenchmarkRemoveMatch
//	BenchmarkRemoveMatch/slice_len_10
//	BenchmarkRemoveMatch/slice_len_10-16         	  491574	      2329 ns/op
//	BenchmarkRemoveMatch/slice_len_100
//	BenchmarkRemoveMatch/slice_len_100-16        	  381914	      3376 ns/op
//	BenchmarkRemoveMatch/slice_len_1000
//	BenchmarkRemoveMatch/slice_len_1000-16       	   37308	     29885 ns/op
//	BenchmarkRemoveMatch/slice_len_10000
//	BenchmarkRemoveMatch/slice_len_10000-16      	     312	   3780946 ns/op
//	BenchmarkRemoveMatch/slice_len_100000
//	BenchmarkRemoveMatch/slice_len_100000-16     	       2	 611002579 ns/op
//	PASS
func BenchmarkRemoveMatch(b *testing.B) {
	sliceLens := []int{10, 100, 1000, 10000, 100_000}
	for _, sliceLen := range sliceLens {
		b.Run(fmt.Sprintf("slice len %d", sliceLen), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				slice := generateSlice(sliceLen)
				b.StartTimer()
				res, removed := sliceutil.RemoveMatch(slice, func(e int) bool {
					return e >= 5
				})
				require.Equal(b, sliceLen/2, len(res))
				require.Equal(b, sliceLen/2, removed)
			}
		})
	}
}

func generateSlice(len int) []int {
	slice := make([]int, len)
	for i, _ := range slice {
		slice[i] = i % 10
	}
	return slice
}
