package fileutil

import (
	"io"
	"os"
	"time"

	"github.com/eluv-io/common-go/util/syncutil"
	"github.com/eluv-io/common-go/util/timeutil"
	"github.com/eluv-io/errors-go"
	elog "github.com/eluv-io/log-go"
)

const (
	tempExt = ".temp"
)

var (
	log            = elog.Get("/eluvio/fileutil")
	logIfWait4Lock = time.Millisecond * 100
	safeFiles      = syncutil.NamedLocks{}
)

// lockSafeFile takes a lock in 'safeFiles' for the given path and returns the
// unlocker that must be called after using the locked file.
// This uses real sync.Mutex. Alternatively we could use:
// - advisory file locking as done by go internals in package src/cmd/go/internal/lockedfile.
// - or: https://github.com/rboyer/safeio
func lockSafeFile(op, path string) syncutil.Unlocker {
	watch := timeutil.StartWatch()
	unlock := safeFiles.Lock(path)

	d := watch.Duration()
	if d > logIfWait4Lock {
		log.Warn(op+" - waited for file lock",
			"path", path,
			"duration", d)
	}
	return unlock
}

func PurgeSafeFile(path string) error {
	unlocker := lockSafeFile("PurgeSafeFile", path)
	defer unlocker.Unlock()

	_ = os.Remove(path + tempExt)
	return os.RemoveAll(path)
}

// NewSafeWriter returns a writer that writes to a temporary file and attempts to replace the target file upon
// finalization. This prevents the target file from becoming corrupted in case of crashes and ensures "atomic writes".
//
// The returned finalize function must be called when writing completes successfully or fails with an error and behaves
// as follows:
//   - if the provided error is nil, it tries to close and rename the temp file and returns any errors that might occur
//     while closing or renaming the temp file
//   - if the provided error is not nil, it tries to close and remove the temp file and returns the provided in error
func NewSafeWriter(path string) (w io.Writer, finalize func(error) error, err error) {
	return newSafeWriter(path)
}
func newSafeWriter(path string, lock ...bool) (w io.Writer, finalize func(error) error, err error) {
	var unlocker syncutil.Unlocker
	if len(lock) > 0 && !lock[0] {
		unlocker = &noopUnlocker{}
	} else {
		unlocker = lockSafeFile("NewSafeWriter", path)
	}

	tmp := path + tempExt
	tmpFile, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return nil, nil, err
	}

	finalize = func(org error) error {
		defer unlocker.Unlock()
		err := tmpFile.Close()
		if org != nil {
			_ = os.Remove(tmp)
			return org
		}
		if err != nil {
			return errors.E("safe write close", errors.K.IO.Default(), err)
		}

		count := 0
		for {
			count++
			_ = os.Remove(path)
			err = os.Rename(tmp, path)

			if err == nil || count > 3 {
				return err
			}

			// retry if renaming fails with 'no such file or directory'
			var lnkErr *os.LinkError
			if ok := errors.As(err, &lnkErr); ok && os.IsNotExist(lnkErr.Err) {
				log.Warn("safe writer - rename failed (no such file)",
					"temp_file", tmp,
					"count", count)
				time.Sleep(time.Millisecond * 2 * time.Duration(count))
				continue
			}

			return errors.E("safe write", errors.K.IO.Default(), err)
		}
	}

	return tmpFile, finalize, nil
}

// NewSafeReader works in conjunction with NewSafeWriter and implements the recovery logic needed for interrupted
// writes.
//   - If the temporary file exists, but no target file, then the temp file is renamed and used
//   - If both the temporary file and the target exists, then the temp file is removed and the target file used.
//   - Otherwise the target file is attempted to be opened
func NewSafeReader(path string) (io.ReadCloser, error) {
	return newSafeReader(path)
}
func newSafeReader(path string, lock ...bool) (io.ReadCloser, error) {
	var unlocker syncutil.Unlocker
	if len(lock) > 0 && !lock[0] {
		unlocker = &noopUnlocker{}
	} else {
		unlocker = lockSafeFile("NewSafeReader", path)
	}

	tmp := path + tempExt
	if _, err := os.Stat(tmp); err == nil {
		fexists := false
		if _, err := os.Stat(path); err == nil {
			fexists = true
		}

		switch fexists {
		case true:
			// both files exist: assume we crashed writing temp or just afterward
			// note: the assumption could be wrong without lock as we could be writing
			_ = os.Remove(tmp)
		case false:
			// temp was written correctly but not renamed to f.path
			// note: the assumption could be wrong without lock as we could be writing
			_ = os.Remove(path)
			err = os.Rename(tmp, path)
			if err != nil {
				return nil, err
			}
		}
	}
	ret, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &safeFile{File: ret, unlocker: unlocker}, nil
}

type noopUnlocker struct{}

func (l *noopUnlocker) Unlock() {}

type safeFile struct {
	*os.File
	unlocker syncutil.Unlocker
}

func (f *safeFile) Close() error {
	defer f.unlocker.Unlock()
	return f.File.Close()
}
