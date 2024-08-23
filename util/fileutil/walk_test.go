package fileutil_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/eluv-io/common-go/util/fileutil"
	"github.com/eluv-io/common-go/util/testutil"

	"github.com/stretchr/testify/require"
)

// folder creates a test folder with a predefined files structure
//
//	 folder
//		|-- t1
//		|   |-- t1f1
//		|   |-- td1
//		|   |   |-- d0 (with zero file)
//		|   |   |-- d1 (with one file)
//		|   |   `-- d2 (with two files)
//		|   `-- td2
//		|       |-- f1   -> td1/d1/f1
//		|       `-- t1f1 -> t1/t1f1
//		`-- t2
//		    `-- f -> folder
func testFolder(t *testing.T, dirPrefix string) (string, func()) {
	dir, cleanup := testutil.TestDir(dirPrefix)

	t1 := filepath.Join(dir, "t1")
	t2 := filepath.Join(dir, "t2")
	require.NoError(t, os.MkdirAll(t1, os.ModePerm))
	require.NoError(t, os.MkdirAll(t2, os.ModePerm))
	err := ioutil.WriteFile(
		filepath.Join(t1, "t1f1"),
		make([]byte, 10),
		os.ModePerm)
	require.NoError(t, err)

	td1 := filepath.Join(dir, "t1", "td1")
	td2 := filepath.Join(dir, "t1", "td2")
	require.NoError(t, os.MkdirAll(td1, os.ModePerm))
	require.NoError(t, os.MkdirAll(td2, os.ModePerm))
	require.NoError(t, os.Symlink(dir, filepath.Join(t2, "f")))

	for i := 0; i < 3; i++ {
		d := filepath.Join(td1, fmt.Sprintf("d%d", i))
		require.NoError(t, os.MkdirAll(d, os.ModePerm))
		for f := 0; f < i; f++ {
			err := ioutil.WriteFile(
				filepath.Join(d, fmt.Sprintf("f%d", f+1)),
				make([]byte, i),
				os.ModePerm)
			require.NoError(t, err)
		}
	}
	require.NoError(t, os.Symlink(
		filepath.Join(td1, "d1", "f1"),
		filepath.Join(td2, "f1")))
	require.NoError(t, os.Symlink(
		filepath.Join(t1, "t1f1"),
		filepath.Join(td2, "t1f1")))
	return dir, cleanup
}

func TestListFile(t *testing.T) {
	folder, cleanup := testFolder(t, "test-list")
	defer cleanup()

	target := filepath.Join(folder, "t1", "td1", "d1", "f0")
	lfs, err := fileutil.ListFiles(target, true)
	require.Error(t, err) // no such file

	target = filepath.Join(folder, "t1", "td1", "d1", "f1")
	lfs, err = fileutil.ListFiles(target, true)
	require.NoError(t, err)
	require.Equal(t, 1, len(lfs))
	require.Equal(t, int64(1), lfs[0].Size)
	require.Equal(t, "", lfs[0].RelPath())

}

func TestListFolder(t *testing.T) {
	folder, cleanup := testFolder(t, "test-list")
	defer cleanup()

	_, err := fileutil.ListFiles(folder, true)
	require.Error(t, err) // circular dep

	target := filepath.Join(folder, "t1", "td1")
	lfs0, err := fileutil.ListFiles(target+"/", true)
	require.NoError(t, err)

	lfs1, err := fileutil.ListFiles(target, true)
	require.NoError(t, err)
	require.EqualValues(t, lfs0, lfs1)

	curr, err := os.Getwd()
	require.NoError(t, err)

	for i := 0; i < 2; i++ {
		require.Equal(t, 7, len(lfs1))
		fis := make([]*fileutil.FileInfo, 0)
		folderCount := 0
		for _, fi := range lfs1 {
			if fi.IsDir {
				folderCount++
			} else {
				fis = append(fis, fi)
			}
		}
		require.Equal(t, 4, folderCount)
		require.Equal(t, 3, len(fis))
		require.Equal(t, "d1/f1", fis[0].RelPath())
		require.Equal(t, int64(1), fis[0].Size)
		require.Equal(t, "d2/f1", fis[1].RelPath())
		require.Equal(t, "d2/f2", fis[2].RelPath())
		require.Equal(t, int64(2), fis[1].Size)
		require.Equal(t, int64(2), fis[2].Size)

		if i == 0 {
			require.NoError(t, os.Chdir(folder))
			lfs1, err = fileutil.ListFiles(filepath.Join("t1", "td1"), true)
			require.NoError(t, err)
		}
	}
	// restore current dir
	require.NoError(t, os.Chdir(curr))

	target = filepath.Join(folder, "t1", "td2")
	lfs2, err := fileutil.ListFiles(target, false)
	require.NoError(t, err)
	require.Equal(t, 1, len(lfs2)) // the root folder only
	require.True(t, lfs2[0].IsDir)

	lfs2, err = fileutil.ListFiles(target, true)
	require.NoError(t, err)
	require.Equal(t, 3, len(lfs2))
	require.True(t, lfs2[0].IsDir)
	require.Equal(t, "f1", lfs2[1].RelPath())
	require.Equal(t, int64(1), lfs2[1].Size)
	require.Equal(t, "t1f1", lfs2[2].RelPath())
	require.Equal(t, int64(10), lfs2[2].Size)

	target = filepath.Join(folder, "t1", "td1", "d2")
	lfs3, err := fileutil.ListFiles(target, false)
	require.NoError(t, err)
	require.Equal(t, 3, len(lfs3)) // includes the directory
	require.Equal(t, "f1", filepath.Base(lfs3[1].Path))
	require.Equal(t, "f2", filepath.Base(lfs3[2].Path))

	lfs3, err = fileutil.ListFiles(target, false, func(path string) bool {
		return filepath.Base(path) == "f2"
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(lfs3))
	require.Equal(t, "f2", filepath.Base(lfs3[0].Path))

}
