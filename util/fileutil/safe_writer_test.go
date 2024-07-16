package fileutil

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeWriter(t *testing.T) {
	testDir, err := os.MkdirTemp("", "TestSafeWriter")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(testDir) }()

	target := filepath.Join(testDir, "f.txt")
	ftmp := target + tempExt

	writer, err := NewSafeWriter(target)
	require.NoError(t, err)

	_, err = writer.Write([]byte("test data"))
	require.NoError(t, err)

	_, err = os.Stat(target)
	require.Error(t, err)

	_, err = os.Stat(ftmp)
	require.NoError(t, err)

	require.NoError(t, writer.CloseWithError(nil))

	_, err = os.Stat(target)
	require.NoError(t, err)

	_, err = os.Stat(ftmp)
	require.Error(t, err)

	bb, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "test data", string(bb))
}

func TestWriteSafe(t *testing.T) {
	testDir, err := os.MkdirTemp("", "TestWriteSafe")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(testDir) }()

	target := filepath.Join(testDir, "f.txt")

	err = WriteSafeFile(target, []byte("test data"), os.ModePerm)
	require.NoError(t, err)

	_, err = os.Stat(target)
	require.NoError(t, err)

	_, err = os.Stat(target + tempExt)
	require.Error(t, err)

	bb, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "test data", string(bb))
}

func TestSafeWriterError(t *testing.T) {
	testDir, err := os.MkdirTemp("", "TestSafeWriterError")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(testDir) }()

	target := filepath.Join(testDir, "f.txt")
	ftmp := target + tempExt

	writer, err := NewSafeWriter(target)
	require.NoError(t, err)

	_, err = writer.Write([]byte("test data"))
	require.NoError(t, err)

	// simulate a write error
	err = writer.CloseWithError(io.ErrShortWrite)
	require.Error(t, err)
	fmt.Println(err)

	_, err = os.Stat(target)
	require.Error(t, err)

	_, err = os.Stat(ftmp)
	require.Error(t, err)
}

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
