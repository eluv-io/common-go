package fileutil

import (
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/eluv-io/errors-go"
)

// NOTES: the implementation of SafeFile comes from https://github.com/google/renameio
// with the following simplifications:
// - no support for writing to a 'temp' directory
// - no support for dealing with umask and existing file permissions
// Also the renameio implementation opens the temporary file with flag os.O_EXCL like this:
// os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, perm)
// We don't use os.O_EXCL as it means we would not be able to reuse the same
// temporary name when restarting after a crash (but this could become an option
// if we were using some randomness in temp files names, like my_file.12345.tmp).
// Finally, an option WithSyncBeforeRename (default is true) was added in case we
// would like to avoid the additional cost of calling sync before renaming.

const (
	// defaultPerm are default permissions for created files
	defaultPerm os.FileMode = 0o666
	// tempExt is the extension added to the file name to create a temporary file
	tempExt = ".temp"
)

// nextRandom is a function generating a random number.
func nextRandom() string {
	return strconv.FormatInt(rand.Int63(), 10)
}

// SafeWriter implements 'safe' writes by writing to a temporary file and renaming
// the temporary file when closing.
type SafeWriter interface {
	io.Writer
	// CloseWithError closes this writer: writeErr is any error that occurred
	// earlier when writing.
	// - when a not nil writeErr is passed, the function cleans up the temporary
	//   file and return the original error.
	// - otherwise, the file is closed and any error closing the file returned.
	CloseWithError(writeErr error) error
}

// WriteSafeFile mirrors os.WriteFile, replacing an existing file with the same
// name atomically.
func WriteSafeFile(filename string, data []byte, perm os.FileMode, opts ...Option) (err error) {
	opts = append([]Option{
		WithPermissions(perm),
	}, opts...)

	t, err := NewPendingFile(filename, opts...)
	if err != nil {
		return err
	}

	defer func() {
		err = t.CloseWithError(err)
	}()

	if _, err := t.Write(data); err != nil {
		return err
	}

	return
}

// PendingFile is a pending temporary file, waiting to replace the destination
// path in a call to CloseAtomicallyReplace.
type PendingFile struct {
	*os.File

	// Note: the temporary file should be removed when an error occurs, but a
	// remove must not be attempted if the rename succeeded, as a new file might
	// have been created with the same name.

	path             string // target path where the final file is expected
	done             bool   // done is set to true when the temporary file was removed either as a result of a successful rename or cleanup
	closed           bool   // closed is set to true when the temporary file was closed
	noRenameOnClose  bool   // option to not rename when closing
	syncBeforeRename bool   // option to 'sync' before renaming (default is true)
}

func NewSafeWriter(path string, opts ...Option) (SafeWriter, error) {
	return NewPendingFile(path, opts...)
}

// NewPendingFile creates a temporary file destined to atomically creating or
// replacing the destination file at path when closing.
func NewPendingFile(path string, opts ...Option) (*PendingFile, error) {
	cfg := config{
		path:             path,
		createPerm:       defaultPerm,
		syncBeforeRename: true,
	}

	for _, o := range opts {
		o.apply(&cfg)
	}

	f, err := openTempFile(cfg.path, tempExt, cfg.createPerm)
	if err != nil {
		return nil, err
	}

	return &PendingFile{
		File:             f,
		path:             cfg.path,
		noRenameOnClose:  cfg.noRenameOnClose,
		syncBeforeRename: cfg.syncBeforeRename,
	}, nil
}

func openTempFile(name, ext string, perm os.FileMode) (*os.File, error) {
	try := 0
	for {
		// note: PurgeSafeFile must be changed if the way fileName is constructed changes
		fileName := name + "." + nextRandom() + ext
		f, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if os.IsExist(err) {
			if try++; try < 10000 {
				continue
			}
			return nil, &os.PathError{Op: "openTempFile", Path: fileName, Err: os.ErrExist}
		}
		return f, err
	}
}

// Path returns the target file.
func (t *PendingFile) Path() string {
	return t.path
}

// Cleanup is a no-op if CloseAtomicallyReplace succeeded, and otherwise closes
// and removes the temporary file. Calling this function should be not necessary
// in normal use, only in case option WithNoRenameOnClose was used.
//
// This method is not safe for concurrent use by multiple goroutines.
func (t *PendingFile) Cleanup() error {
	if t.done {
		return nil
	}
	// An error occurred. Close and remove the tempfile. Errors are returned for
	// reporting, there is nothing the caller can recover here.
	var closeErr error
	if !t.closed {
		t.closed = true
		closeErr = t.File.Close()
	}
	if err := os.Remove(t.Name()); err != nil {
		return err
	}
	t.done = true
	return closeErr
}

// closeAtomicallyReplace closes the temporary file and atomically replaces
// the destination file with it, i.e., a concurrent open(2) call will either
// open the file previously located at the destination path (if any), or the
// just written file, but the file will always be present.
//
// This method is not safe for concurrent use by multiple goroutines.
func (t *PendingFile) closeAtomicallyReplace() error {
	// -- comment from renameio original code --
	// Even on an ordered file system (e.g. ext4 with data=ordered) or file
	// systems with write barriers, we cannot skip the fsync(2) call as per
	// Theodore Ts'o (ext2/3/4 lead developer):
	//
	// > data=ordered only guarantees the avoidance of stale data (e.g., the previous
	// > contents of a data block showing up after a crash, where the previous data
	// > could be someone's love letters, medical records, etc.). Without the fsync(2)
	// > a zero-length file is a valid and possible outcome after the rename.
	if t.syncBeforeRename {
		if err := t.Sync(); err != nil {
			return err
		}
	}
	t.closed = true
	if err := t.File.Close(); err != nil {
		return err
	}
	if err := os.Rename(t.Name(), t.path); err != nil {
		return err
	}
	t.done = true
	return nil
}

// Close closes the file.
// When configured with WithNoRenameOnClose, it just calls Close() on the
// underlying file, otherwise, it calls closeAtomicallyReplace().
func (t *PendingFile) Close() error {
	if t.noRenameOnClose {
		t.closed = true
		return t.File.Close()
	}
	return t.closeAtomicallyReplace()
}

// CloseWithError closes the file if writeErr is nil otherwise it calls Cleanup
// and returns the original error.
func (t *PendingFile) CloseWithError(writeErr error) error {
	if writeErr != nil {
		_ = t.Cleanup()
		return writeErr
	}
	return t.Close()
}

type config struct {
	path             string
	createPerm       os.FileMode
	syncBeforeRename bool
	noRenameOnClose  bool
}

// Option is the interface implemented by all configuration function return values.
type Option interface {
	apply(*config)
}

type optionFunc func(*config)

func (fn optionFunc) apply(cfg *config) {
	fn(cfg)
}

// WithPermissions sets the permissions for the target file.
func WithPermissions(perm os.FileMode) Option {
	perm &= os.ModePerm
	return optionFunc(func(cfg *config) {
		cfg.createPerm = perm
	})
}

// WithSyncBeforeRename configure safe file to do a sync before renaming,
// default is true.
func WithSyncBeforeRename(sync bool) Option {
	return optionFunc(func(cfg *config) {
		cfg.syncBeforeRename = sync
	})
}

// WithNoRenameOnClose causes PendingFile.Close() to actually not call CloseAtomicallyReplace()
// and just call Close on its temporary file.
// Use of this option is discouraged since when using it, the caller has to
// handle renaming the temporary file to the actual target path and/or calling
// Cleanup - as well as manage potential garbage left behind (see PurgeSafeFile).
func WithNoRenameOnClose() Option {
	return optionFunc(func(c *config) {
		c.noRenameOnClose = true
	})
}

// PurgeSafeFile removes the file at the given path as well as all temporary files
// that could have been created when opening a PendingFile for the given path.
// Calling this function should not be necessary in a normal use of SafeWriter.
func PurgeSafeFile(path string) error {
	count := 0
	dir := filepath.Dir(path)
	if tempFiles, err := fs.Glob(os.DirFS(dir), filepath.Base(path)+".*"+tempExt); err == nil {
		for _, tmp := range tempFiles {
			_ = os.Remove(filepath.Join(dir, tmp))
		}
	}
	for {
		count++
		err := os.Remove(path)
		if os.IsNotExist(err) {
			err = nil
		}
		if err != nil {
			return err
		}
		if _, err = os.Stat(path); err == nil {
			if count > 3 {
				return errors.E("PurgeSafeFile", errors.K.IO,
					"reason", fmt.Sprintf("file not deleted after %d attempts", count),
					"path", path)
			}
			time.Sleep(time.Millisecond * 10 * time.Duration(count*2))
			continue
		}
		break
	}

	return nil
}
