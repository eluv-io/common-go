package assertions

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/errors-go"
)

// FilePathsEqual asserts that the two file paths have the same content. Directories are traversed recursively,
// ensuring they contain the same files and sub-directories. Files are compared by reading and comparing their content.
func FilePathsEqual(t require.TestingT, d1, d2 string) {
	err := filepath.Walk(d1, func(path string, info fs.FileInfo, err error) error {
		require.NoError(t, err)

		relPath := path[len(d1):]
		tgt := filepath.Join(d2, relPath)

		if info.IsDir() {
			stat, err := os.Stat(tgt)
			require.NoError(t, err)
			require.True(t, stat.IsDir())
		} else {
			FileContentsEqual(t, path, tgt)
		}

		return nil
	})
	require.NoError(t, err)

	// walk the other dir
	d1, d2 = d2, d1
	err = filepath.Walk(d1, func(path string, info fs.FileInfo, err error) error {
		require.NoError(t, err)

		relPath := path[len(d1):]
		tgt := filepath.Join(d2, relPath)

		// this time comparing file content is not needed, just make sure the file/dir exists
		stat, err := os.Stat(tgt)
		require.NoError(t, err)
		require.Equal(t, info.IsDir(), stat.IsDir())

		return nil
	})
	require.NoError(t, err)
}

// FileContentsEqual asserts that the content of two files is equal.
func FileContentsEqual(t require.TestingT, f1, f2 string) {
	fh1, err := os.Open(f1)
	require.NoError(t, err)
	defer errors.Ignore(fh1.Close)

	fh2, err := os.Open(f2)
	require.NoError(t, err)
	defer errors.Ignore(fh2.Close)

	buf1 := make([]byte, 64*1024)
	buf2 := make([]byte, 64*1024)
	for {
		n1, err1 := io.ReadFull(fh1, buf1)
		n2, err2 := io.ReadFull(fh2, buf2)
		require.Equal(t, n1, n2)
		require.Equal(t, err1, err2)
		require.Equal(t, buf1, buf2)
		if err1 != nil {
			break
		}
	}
}
