package fileutil

import (
	"os"
	"path/filepath"

	"github.com/qluvio/content-fabric/errors"
)

type FileInfo struct {
	// target path + rel path under target path
	// e.g. target path=./tmp
	//      file found at ./tmp/a/file
	//      ==> path = ./tmp/a/file
	Path string
	// the base path that is searched
	TargetPath string
	// true if directory, false is file
	IsDir bool
	// size of the file
	Size int64
}

func (i *FileInfo) RelPath() string {
	if i.TargetPath == i.Path {
		return ""
	}
	rel, _ := filepath.Rel(i.TargetPath, i.Path)
	//return i.Path[len(i.TargetPath):] --> produces '/something' (not relative)
	return rel
}

// ListFiles lists the files at target path.
//
// * targetPath: the target path
// * followSymLinks: true to follow symlinks
// * filter: an optional filter function to accept files. All files are accepted
//   when not provided
// Return
// * an array of *FileInfo describing the listed files. Symlinks are not
//   included if followSymLinks is false.
// * error is not nil in case followSymLinks is true and a loop is detected
//   or if there was an error listing the content of a directory.
func ListFiles(targetPath string, followSymLinks bool, filter ...func(path string) bool) ([]*FileInfo, error) {
	var res []*FileInfo

	targetPath = filepath.Clean(targetPath)
	err := Walk(targetPath, followSymLinks, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !followSymLinks && info.Mode()&os.ModeSymlink == os.ModeSymlink {
			// don't include symlinks if we don't follow
			return nil
		}
		if len(filter) > 0 && filter[0] != nil && !filter[0](path) {
			return nil
		}
		fi := &FileInfo{
			Path:       path,
			TargetPath: targetPath,
			IsDir:      info.IsDir(),
			Size:       info.Size(),
		}
		res = append(res, fi)
		return nil
	})
	if err != nil {
		return nil, errors.E("ListFiles", err)
	}

	return res, nil
}
