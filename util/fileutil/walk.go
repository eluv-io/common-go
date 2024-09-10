package fileutil

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/eluv-io/errors-go"
)

// Walk implements filepath.Walk but allows to also resolve symlinks
func Walk(path string, followSymlinks bool, walkFn filepath.WalkFunc) error {
	if !followSymlinks {
		return filepath.Walk(path, walkFn)
	}
	return walk(path, path, walkFn, nil)
}

// walk walks the given walkPath.
//
//   - walkPath: path with which the walk starts
//   - linkPath: the path of the directory symlink that triggered this walk, or
//     same as walkPath for original walk
//   - walkFn:   the function called for all found files and directories
//   - visited:  list of walkPaths already visited
func walk(walkPath string, linkPath string, walkFn filepath.WalkFunc, visited []string) error {
	e := errors.Template("fileutil.WalkSym", "walk_path", walkPath)
	for _, v := range visited {
		if strings.HasPrefix(walkPath, v) {
			return e(errors.K.Invalid,
				"reason", "symlink loop detected",
				"symlink", linkPath,
				"already_visited", v)
		}
	}
	visited = append(visited, walkPath)

	symWalkFunc := func(path string, info os.FileInfo, err error) error {
		e := e.Add("path", path)
		if fname, err := filepath.Rel(walkPath, path); err == nil {
			path = filepath.Join(linkPath, fname)
		} else {
			return e(err)
		}

		if err == nil && info.Mode()&os.ModeSymlink == os.ModeSymlink {
			finalPath, err := filepath.EvalSymlinks(path)
			if err != nil {
				return e(err)
			}
			info, err = os.Lstat(finalPath)
			if err != nil {
				return walkFn(path, info, err)
			}
			if info.IsDir() {
				return walk(finalPath, path, walkFn, visited)
			}
		}

		return walkFn(path, info, err)
	}
	return e.IfNotNil(filepath.Walk(walkPath, symWalkFunc))
}
