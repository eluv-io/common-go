package ctxutil_test

import (
	"context"
	"testing"

	"github.com/eluv-io/common-go/util/ctxutil"
)

// BenchmarkContextStack measures the context stack with all goroutines using it at once. Run it with -cpu 1,8,32 to see
// the contention on the stack's map.
func BenchmarkContextStack(b *testing.B) {
	b.Run("Ctx", func(b *testing.B) {
		cs := ctxutil.NewStack()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			defer cs.Push(context.Background())()
			for pb.Next() {
				_ = cs.Ctx()
			}
		})
	})
	b.Run("PushPop", func(b *testing.B) {
		cs := ctxutil.NewStack()
		ctx := context.Background()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				cs.Push(ctx)()
			}
		})
	})
}
