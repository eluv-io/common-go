package fileutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeReader(t *testing.T) {
	testDir, err := os.MkdirTemp("", "TestSafeReaderWriter")
	require.NoError(t, err)

	type testCase struct {
		name       string
		original   string
		temp       string
		expectFail bool
		expectLoad string
	}
	for _, tc := range []*testCase{
		{name: "not_found", expectFail: true},
		{name: "regular_success", original: "version_0", temp: "", expectLoad: "version_0"},
		{name: "crash_after_rm", original: "", temp: "version_1", expectLoad: "version_1"},
		{name: "crash_before_rm", original: "version_0", temp: "versi", expectLoad: "version_0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(testDir, tc.name)
			err = os.Mkdir(dir, os.ModePerm)
			require.NoError(t, err, tc.name)
			target := filepath.Join(dir, "f.txt")
			ftmp := target + tempExt

			if len(tc.original) > 0 {
				err = os.WriteFile(target, []byte(tc.original), os.ModePerm)
				require.NoError(t, err, tc.name)
			}
			if len(tc.temp) > 0 {
				err = os.WriteFile(ftmp, []byte(tc.temp), os.ModePerm)
				require.NoError(t, err, tc.name)
			}

			reader, err := NewSafeReader(target)
			if tc.expectFail {
				require.Error(t, err, tc.name)
				return
			}
			res, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, tc.expectLoad, string(res))
			_, err = os.Stat(ftmp)
			require.Error(t, err)
			require.NoError(t, reader.Close())
		})
	}

	_ = os.RemoveAll(testDir)
}

func TestSafeWriter(t *testing.T) {
	testDir, err := os.MkdirTemp("", "TestSafeReaderWriter")
	require.NoError(t, err)

	target := filepath.Join(testDir, "f.txt")
	ftmp := target + tempExt

	writer, finalize, err := NewSafeWriter(target)
	require.NoError(t, err)

	_, err = writer.Write([]byte("test data"))
	require.NoError(t, err)

	_, err = os.Stat(target)
	require.Error(t, err)

	_, err = os.Stat(ftmp)
	require.NoError(t, err)

	require.NoError(t, finalize(nil))

	_, err = os.Stat(target)
	require.NoError(t, err)

	_, err = os.Stat(ftmp)
	require.Error(t, err)
}

func TestSafeWriterError(t *testing.T) {
	testDir, err := os.MkdirTemp("", "TestSafeReaderWriter")
	require.NoError(t, err)

	target := filepath.Join(testDir, "f.txt")
	ftmp := target + tempExt

	writer, finalize, err := NewSafeWriter(target)
	require.NoError(t, err)

	_, err = writer.Write([]byte("test data"))
	require.NoError(t, err)

	// simulate a write error
	err = finalize(io.ErrShortWrite)
	require.Error(t, err)
	fmt.Println(err)

	_, err = os.Stat(target)
	require.Error(t, err)

	_, err = os.Stat(ftmp)
	require.Error(t, err)
}
