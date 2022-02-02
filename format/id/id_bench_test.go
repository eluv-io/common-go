package id_test

import (
	"testing"

	"github.com/eluv-io/common-go/format/id"
)

// Baseline performance:
//
// goos: darwin
// goarch: amd64
// pkg: github.com/eluv-io/common-go/format/id
// cpu: Intel(R) Core(TM) i9-9980HK CPU @ 2.40GHz
// BenchmarkId
// BenchmarkId/string(id)
// BenchmarkId/string(id)-16         	225890617	         4.986 ns/op
// BenchmarkId/id.String()
// BenchmarkId/id.String()-16        	 2496332	       443.5 ns/op
// BenchmarkId/noop
// BenchmarkId/noop-16               	1000000000	         0.2818 ns/op
func BenchmarkId(b *testing.B) {
	anID := id.Generate(id.User)
	b.Run("string(id)", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = string(anID.Bytes())
		}
	})
	b.Run("id.String()", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = anID.String()
		}
	})
	b.Run("noop", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
		}
	})
}
