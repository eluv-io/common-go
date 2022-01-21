package aferoutil

import (
	"io"
	"os"
	"path/filepath"

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
