package fileutil

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/errors-go"
)

func TestSafeWriter(t *testing.T) {
	testDir, err := os.MkdirTemp("", "TestSafeWriter")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(testDir) }()

	target := filepath.Join(testDir, "f.txt")

	writer, err := NewSafeWriter(target)
	require.NoError(t, err)

	_, err = writer.Write([]byte("test data"))
	require.NoError(t, err)

	_, err = os.Stat(target)
	require.Error(t, err)

	pf, ok := writer.(*PendingFile)
	require.True(t, ok)
	ftmp := pf.File.Name()
	require.True(t, strings.HasSuffix(ftmp, tempExt))
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
	dirFs := os.DirFS(testDir)
	temps, err := fs.Glob(dirFs, "*"+tempExt)
	require.NoError(t, err)
	require.Equal(t, 0, len(temps))

	bb, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "test data", string(bb))
}

func TestSafeWriterError(t *testing.T) {
	testDir, err := os.MkdirTemp("", "TestSafeWriterError")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(testDir) }()

	target := filepath.Join(testDir, "f.txt")
	writer, err := NewSafeWriter(target)
	require.NoError(t, err)
	pf, ok := writer.(*PendingFile)
	require.True(t, ok)
	ftmp := pf.File.Name()

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

func TestPurgeSafeFile(t *testing.T) {
	testDir, err := os.MkdirTemp("", "TestPurgeSafeFile")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(testDir) }()

	target := filepath.Join(testDir, "target")
	for _, fileName := range []string{
		"target",
		"target.0" + tempExt,
		"target.1234567890" + tempExt,
		"target" + tempExt,
		"other",
		"other.123" + tempExt,
	} {
		err = os.WriteFile(filepath.Join(testDir, fileName), []byte("this is a sample"), os.ModePerm)
		require.NoError(t, err)
	}

	err = PurgeSafeFile(target)
	require.NoError(t, err)

	files, err := fs.Glob(os.DirFS(testDir), "*")
	require.NoError(t, err)

	require.Equal(t, 3, len(files))
	require.Equal(t, "other", files[0])
	require.Equal(t, "other.123"+tempExt, files[1])
	require.Equal(t, "target"+tempExt, files[2])
}

func TestSafeWriterWithConcurrentRead(t *testing.T) {
	testDir, err := os.MkdirTemp("", "TestSafeWriterConcurrent")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(testDir) }()

	target := filepath.Join(testDir, "f.txt")

	readFile := func(f *os.File) string {
		defer errors.Ignore(f.Close)
		bb, err := io.ReadAll(f)
		require.NoError(t, err)
		return string(bb)
	}

	cases := []string{
		"body-0",
		"buddy-x",
		"other than that",
		"or what",
		"but not me",
	}
	for i, tc := range cases {
		f, err := os.Open(target)
		if i == 0 {
			require.True(t, os.IsNotExist(err))
		} else {
			require.NoError(t, err)
		}
		pf, err := NewPendingFile(target)
		require.NoError(t, err)

		// content is what was written in the previous step, if we have a 'f'
		if f != nil && i%2 == 0 {
			s := readFile(f)
			require.Equal(t, cases[i-1], s)
		}

		_, err = pf.WriteString(tc)
		require.NoError(t, err)
		err = pf.CloseWithError(err)
		require.NoError(t, err)

		// content is what was written in the previous step, if we have a 'f'
		// even if a new version was written
		if f != nil && i%2 != 0 {
			s := readFile(f)
			require.Equal(t, cases[i-1], s)
		}
	}

	files, err := fs.Glob(os.DirFS(testDir), "*")
	require.NoError(t, err)

	require.Equal(t, 1, len(files))
	require.Equal(t, "f.txt", files[0])
}
