package aferoutil

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/djherbis/times"
	"github.com/spf13/afero"

	"github.com/eluv-io/errors-go"

	"github.com/eluv-io/common-go/util/byteutil"
	"github.com/eluv-io/common-go/util/stringutil"
)

// MoveFile moves the given source file to the destination path. First, the file is attempted to be renamed. If that
// fails, the files' data is copied to the destination path.
func MoveFile(fs afero.Fs, src, dst string) error {
	e := errors.Template("MoveFile", errors.K.Invalid, "src", src, "dst", dst)
	if src == "" {
		return e("reason", "empty source path")
	}
	if dst == "" {
		return e("reason", "empty destination path")
	}

	stat, err := fs.Stat(src)
	if err != nil {
		return e(errors.K.IO, "reason", "cannot stat source")
	}
	if stat.IsDir() {
		return e(errors.K.Invalid, "reason", "source is a directory")
	}

	dstDir := filepath.Dir(dst)
	stat, err = fs.Stat(dstDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return e(errors.K.IO, err, "reason", "failed to stat destination dir")
		}
		err = fs.MkdirAll(dstDir, os.ModePerm)
		if err != nil {
			return e(errors.K.IO, err, "reason", "failed to create destination dir")
		}
	} else if !stat.IsDir() {
		return e("reason", "destination is not a directory")
	}

	err = fs.Rename(src, dst)
	if err == nil {
		return nil
	}

	if os.IsNotExist(err) {
		return e(errors.K.NotExist, err)
	}

	// try copying
	fdSrc, err := fs.Open(src)
	if err != nil {
		return e(errors.K.IO, err, "reason", "failed to open source file")
	}
	defer errors.Ignore(fdSrc.Close)

	fdDst, err := fs.Create(dst)
	if err != nil {
		return e(errors.K.IO, err, "reason", "failed to create destination file")
	}
	defer errors.Ignore(fdDst.Close)

	_, err = io.Copy(fdDst, fdSrc)
	if err != nil {
		return e(errors.K.IO, err, "reason", "failed to copy file data")
	}

	err = fs.Remove(src)
	if err != nil {
		return e(errors.K.IO, err, "reason", "failed to remove source file")
	}

	return nil
}

// RecreateDir re-creates the given directory and all sub-directories to reduce filesystem overhead for the directories
// (see https://github.com/openzfs/zfs/issues/4933). Returns the number of moved files. If newFilePathFn is specified,
// visited files will be moved to newFilePathFn(filePath) relative to the given top directory; otherwise, files will be
// moved to the matching path in the re-created directory. If newFilePathFn returns an empty string for a visited file,
// the file will be removed instead of moved. Sub-directories in the given excludeDirs and empty sub-directories will
// not be traversed but treated as a visited file and moved accordingly.
// Note: Uses the ".recreate" file extension for interim directories so that RecreateDir can be retried upon failures
// and resume progress
func RecreateDir(fs afero.Fs, path string, newFilePathFn func(string, bool) string, excludeDirs ...string) (int, error) {
	e := errors.Template("RecreateDir", errors.K.IO, "path", path)
	if newFilePathFn == nil {
		newFilePathFn = func(p string, _ bool) string {
			return p
		}
	}
	newPathFn := func(p string, isDir bool) string {
		newPath := newFilePathFn(p, isDir)
		if newPath == "" {
			return ""
		}
		return filepath.Join(path, newPath)
	}
	// Move existing/old dir, create new dir, move files over, delete old dir
	backupPath := path + ".recreate"
	_, err := fs.Stat(backupPath) // Check in case backup dir already exists from previously failed attempt
	if os.IsNotExist(err) {
		err := fs.Rename(path, backupPath)
		if err != nil {
			return 0, e(err)
		}
	} else if err != nil {
		return 0, e(err)
	}
	err = fs.MkdirAll(path, os.ModePerm)
	if err != nil {
		return 0, e(err)
	}
	var visitDir func(string, string) (int, error)
	visitDir = func(basePath string, subPath string) (int, error) {
		n := 0
		path := filepath.Join(basePath, subPath)
		dir, err := fs.Open(path)
		if err != nil {
			return n, e(err)
		}
		defer errors.Ignore(dir.Close)
		for {
			files, err := dir.Readdir(4096)
			for _, file := range files {
				filePath := filepath.Join(subPath, file.Name())
				var err error
				if file.IsDir() && !slices.Contains(excludeDirs, filePath) {
					var m int
					m, err = visitDir(basePath, filePath)
					n += m
					if err == nil && m == 0 {
						newPath := newPathFn(filePath, true)
						if newPath != "" {
							err = fs.MkdirAll(newPath, os.ModePerm)
							if err == nil {
								n++
							}
						}
					}
				} else {
					oldPath := filepath.Join(basePath, filePath)
					newPath := newPathFn(filePath, file.IsDir())
					if newPath != "" {
						err = fs.MkdirAll(filepath.Dir(newPath), os.ModePerm)
						if err == nil {
							err = fs.Rename(oldPath, newPath)
							if err == nil {
								n++
							}
						}
					} else {
						err = fs.RemoveAll(oldPath)
					}
				}
				if err != nil {
					return n, e(err)
				}
			}
			if err == io.EOF {
				break
			} else if err != nil {
				return n, e(err)
			}
		}
		errors.Ignore(dir.Close)
		err = fs.Remove(path)
		if err != nil {
			return n, e(err)
		}
		return n, nil
	}
	return visitDir(backupPath, "")
}

// CheckAccessTimeEnabled checks whether access_time is enabled in the filesystem at the given path.
func CheckAccessTimeEnabled(fs afero.Fs, path string) (bool, error) {
	e := errors.Template("CheckAccessTimeEnabled", errors.K.IO, "path", path)
	atn := filepath.Join(path, "atc"+stringutil.RandomString(8))
	var ati os.FileInfo
	atf, err := fs.Create(atn)
	if err == nil {
		ati, err = atf.Stat()
		if err == nil {
			_, err = atf.Write(byteutil.RandomBytes(1024))
			if err == nil {
				err = atf.Close()
			}
		}
	}
	defer func(atf afero.File) {
		if atf != nil {
			_ = atf.Close()
			_ = fs.Remove(atn)
		}
	}(atf)
	if err != nil {
		return false, e(err)
	}
	if ati.Sys() == nil {
		return false, e(errors.K.NotImplemented, "reason", "system stats not available", "path", atn)
	}
	at1 := times.Get(ati).AccessTime()
	time.Sleep(time.Millisecond * 5)
	atf, err = fs.Open(atn)
	if err == nil {
		_, err = io.ReadAll(atf)
		if err == nil {
			err = atf.Close()
		}
	}
	defer func(atf afero.File) {
		if atf != nil {
			_ = atf.Close()
		}
	}(atf)
	if err != nil {
		return false, e(err)
	}
	ati, err = fs.Stat(atn)
	if err != nil {
		return false, e(err)
	}
	at2 := times.Get(ati).AccessTime()
	return !at1.Equal(at2), nil
}
