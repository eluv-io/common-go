package fileutil

import (
	"io"
	"os"

	"github.com/eluv-io/errors-go"
)

const (
	tempExt = ".temp"
)

// NewSafeWriter returns a writer that writes to a temporary file and attempts to replace the target file upon
// finalization. This prevents the target file from becoming corrupted in case of crashes and ensures "atomic writes".
//
// The returned finalize function must be called when writing completes successfully or fails with an error and behaves
// as follows:
//   - if the provided error is nil, it tries to close and rename the temp file and returns any errors that might occur
//     while closing or renaming the temp file
//   - if the provided error is not nil, it tries to close and remove the temp file and returns the provided in error
func NewSafeWriter(path string) (w io.Writer, finalize func(error) error, err error) {
	tmp := path + tempExt
	tmpFile, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return nil, nil, err
	}

	finalize = func(org error) error {
		err := tmpFile.Close()
		if org != nil {
			_ = os.Remove(tmp)
			return org
		}
		if err == nil {
			_ = os.Remove(path)
			err = os.Rename(tmp, path)
		}
		if err == nil {
			return nil
		}
		return errors.E("safe write", errors.K.IO.Default(), err)
	}

	return tmpFile, finalize, nil
}

// NewSafeReader works in conjunction with NewSafeWriter and implements the recovery logic needed for interrupted
// writes.
//   - If the temporary file exists, but no target file, then the temp file is renamed and used
//   - If both the temporary file and the target exists, then the temp file is removed and the target file used.
//   - Otherwise the target file is attempted to be opened
func NewSafeReader(path string) (io.ReadCloser, error) {
	tmp := path + tempExt
	if _, err := os.Stat(tmp); err == nil {
		fexists := false
		if _, err := os.Stat(path); err == nil {
			fexists = true
		}

		switch fexists {
		case true:
			// both files exist: assume we crashed writing temp or just afterward
			_ = os.Remove(tmp)
		case false:
			// temp was written correctly but not renamed to f.path
			_ = os.Remove(path)
			err = os.Rename(tmp, path)
			if err != nil {
				return nil, err
			}
		}
	}
	return os.Open(path)
}
