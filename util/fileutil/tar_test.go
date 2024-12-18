package fileutil_test

import (
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util"
	"github.com/eluv-io/common-go/util/assertions"
	"github.com/eluv-io/common-go/util/fileutil"
	"github.com/eluv-io/common-go/util/testutil"
	"github.com/eluv-io/errors-go"
)

func TestArchiveExtract(t *testing.T) {
	dir, cleanup := testutil.TestDir("compress-decompress")
	defer cleanup()

	srcDir := filepath.Join(dir, "src")
	dstDir := filepath.Join(dir, "dst")
	archive := filepath.Join(dir, "archive.tar.gz")

	createTestFiles(t, srcDir)

	err := fileutil.Archive(srcDir, archive)
	require.NoError(t, err)

	err = fileutil.Extract(archive, dstDir)
	require.NoError(t, err)

	util.PrintDirectoryTree(dir)
	assertions.FilePathsEqual(t, srcDir, dstDir)
}

func createTestFiles(t *testing.T, dir string) {
	createTestFile(t, filepath.Join(dir, "top-10"), 10)
	createTestFile(t, filepath.Join(dir, "top-20"), 20)

	createTestFile(t, filepath.Join(dir, "dir-a", "a-0"), 0)
	createTestFile(t, filepath.Join(dir, "dir-a", "a-10"), 10)
	createTestFile(t, filepath.Join(dir, "dir-a", "a-99"), 99)

	createTestFile(t, filepath.Join(dir, "dir-b", "b-0"), 1)
	createTestFile(t, filepath.Join(dir, "dir-b", "b-125"), 25)
	createTestFile(t, filepath.Join(dir, "dir-b", "b-57"), 57)

	createTestFile(t, filepath.Join(dir, "dir-b", "dir-c", "c-30"), 30)
	createTestFile(t, filepath.Join(dir, "dir-b", "dir-c", "c-40"), 40)
}

func createTestFile(t *testing.T, path string, size int64) {
	err := os.MkdirAll(filepath.Dir(path), 0755)
	require.NoError(t, err)

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0755)
	require.NoError(t, err)
	defer errors.Ignore(f.Close)

	n, err := io.Copy(f, io.LimitReader(rand.Reader, size))
	require.NoError(t, err)
	require.Equal(t, n, size)
}
