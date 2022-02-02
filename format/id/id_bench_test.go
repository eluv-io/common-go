package id_test

import (
	"testing"

	"github.com/eluv-io/common-go/format/id"
)

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
