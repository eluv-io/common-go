package fileutil

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// BenchmarkSafeWriter-8         184   6993383 ns/op	    1127 B/op	      18 allocs/op
func BenchmarkSafeWriter(b *testing.B) {
	doBenchmarkSafeWriter(b, true)
}

// BenchmarkSafeWriterNoSync-8  3296    321936 ns/op	    1134 B/op	      18 allocs/op
func BenchmarkSafeWriterNoSync(b *testing.B) {
	doBenchmarkSafeWriter(b, false)
}

func doBenchmarkSafeWriter(b *testing.B, syncBeforeRename bool) {
	testDir, err := os.MkdirTemp("", "BenchmarkSafeWriter")
	require.NoError(b, err)
	defer func() { _ = os.RemoveAll(testDir) }()

	bb := make([]byte, 1024)
	_, err = rand.Read(bb)
	require.NoError(b, err)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err = WriteSafeFile(
			filepath.Join(testDir, fmt.Sprintf("f%04d", i)),
			bb,
			os.ModePerm,
			WithSyncBeforeRename(syncBeforeRename))
		require.NoError(b, err)
	}
}
