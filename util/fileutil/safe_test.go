package fileutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/errors-go"
)

func TestSafeReader(t *testing.T) {
	testDir, err := os.MkdirTemp("", "TestSafeReader")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(testDir) }()

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
	defer func() { _ = os.RemoveAll(testDir) }()

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
	defer func() { _ = os.RemoveAll(testDir) }()

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

func TestSlowSafeReaderWriter(t *testing.T) {
	testDir, err := os.MkdirTemp("", "TestSlowSafeReaderWriter")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(testDir) }()

	target := filepath.Join(testDir, "f.txt")
	writer, finalize, err := NewSafeWriter(target)
	require.NoError(t, err)
	_, err = writer.Write([]byte("test data"))
	require.NoError(t, err)
	err = finalize(nil)
	require.NoError(t, err)

	// take the lock on target and do a 'slow read'
	reader, err := NewSafeReader(target)
	require.NoError(t, err)
	go func(rd io.ReadCloser) {
		time.Sleep(time.Millisecond * 500)
		_ = rd.Close()
	}(reader)

	// attempt to write during the slow read (log should report a warning)
	writer, finalize, err = NewSafeWriter(target)
	require.NoError(t, err)
	_, err = writer.Write([]byte("test data2"))
	require.NoError(t, err)
	err = finalize(nil)
	require.NoError(t, err)

	reader, err = NewSafeReader(target)
	require.NoError(t, err)
	defer errors.Ignore(reader.Close)
	bb, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "test data2", string(bb))
}

// TestConcurrentSafeReaderWriter shows that reading concurrently with a write
// might lead to fail writing whenever not using lock.
func TestConcurrentSafeReaderWriter(t *testing.T) {
	testDir, err := os.MkdirTemp("", "TestConcurrentSafeReaderWriter")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(testDir) }()

	fmt.Println("test_dir", testDir)
	target := filepath.Join(testDir, "f.txt")

	readFile := func(lock bool) (string, error) {
		reader, err := newSafeReader(target, lock)
		if err != nil {
			return "", err
		}
		bb, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			return "", err
		}
		return string(bb), nil
	}

	type testCase struct {
		lock     bool
		wantFail bool
	}

	for _, tc := range []*testCase{
		{lock: true},
		{lock: false, wantFail: true},
	} {
		//fmt.Println("case", tc.lock)
		tcase := fmt.Sprintf("lock: %v", tc.lock)
		err := PurgeSafeFile(target)
		require.NoError(t, err, tcase)

		writer, finalize, err := newSafeWriter(target, tc.lock)
		require.NoError(t, err, tcase)

		// reading concurrently without lock deletes the '.temp' file
		val := ""
		var rerr error
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func(lock bool) {
			defer wg.Done()
			val, rerr = readFile(lock)
		}(tc.lock)

		time.Sleep(time.Millisecond * 100)
		_, err = writer.Write([]byte("test data"))
		require.NoError(t, err, tcase)
		time.Sleep(time.Millisecond * 100)
		err = finalize(nil)
		wg.Wait()

		require.NoError(t, rerr, tcase)

		if tc.wantFail {
			require.Error(t, err, tcase)
			require.Equal(t, "", val, tcase)
		} else {
			require.NoError(t, err, tcase)
			require.Equal(t, "test data", val, tcase)
		}

		// finalization of the safe writer was previously doing
		// 			_ = os.Remove(path)
		//			err = os.Rename(tmp, path)
		// while now just
		//			err = os.Rename(tmp, path)
		// therefore, we were previously getting an error 'no such file' when reading here
		// while now we have no error (even though the writer failed) because renaming was
		// done by the earlier read!
		ret, err := readFile(tc.lock)
		require.NoError(t, err, tcase)
		require.Equal(t, "test data", ret, tcase)

	}

}
