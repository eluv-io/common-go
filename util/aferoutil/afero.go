package aferoutil

import (
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/eluv-io/errors-go"
	"github.com/spf13/afero"
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
// (see https://github.com/openzfs/zfs/issues/4933). If newFilePathFn is specified, visited files will be moved to
// newFilePathFn(filePath) relative to the given top directory; otherwise, files will be moved to the matching path in
// the re-created directory. Returns the number of moved files. Sub-directories in the given excludeDirs will not be
// traveresed but simply moved to the re-created directory.
// Note: Uses the ".bak" file extension for interim directories so that RecreateDir can be retried upon failures and
// resume progress
func RecreateDir(fs afero.Fs, path string, newFilePathFn func(string) string, excludeDirs ...string) (int, error) {
	e := errors.Template("RecreateDir", errors.K.IO, "path", path)
	var newPathFn func(string) string
	if newFilePathFn == nil {
		newPathFn = func(p string) string {
			return filepath.Join(path, p)
		}
	} else {
		newPathFn = func(p string) string {
			return filepath.Join(path, newFilePathFn(p))
		}
	}
	// Move existing/old dir, create new dir, move files over, delete old dir
	bakPath := path + ".bak"
	_, err := fs.Stat(bakPath) // Check in case backup dir already exists from previously failed attempt
	if os.IsNotExist(err) {
		err := fs.Rename(path, bakPath)
		if err != nil {
			return 0, e(err)
		}
	} else if err != nil {
		return 0, e(err)
	}
	err = fs.Mkdir(path, os.ModePerm)
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
				} else {
					oldPath := filepath.Join(basePath, filePath)
					newPath := newPathFn(filePath)
					err = fs.MkdirAll(filepath.Dir(newPath), os.ModePerm)
					if err == nil {
						err = fs.Rename(oldPath, newPath)
						if err == nil {
							n++
						}
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
		err = fs.Remove(path)
		if err != nil {
			return n, e(err)
		}
		return n, nil
	}
	return visitDir(bakPath, "")
}
