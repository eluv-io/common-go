package testutil

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/eluv-io/log-go"
)

// TestDir creates a new, unique test directory with the given prefix in default
// temp directory of the platform. It returns the directory's path and a cleanup
// function to remove the directory after the test.
func TestDir(prefix string) (path string, cleanup func()) {
	path, err := ioutil.TempDir(os.TempDir(), prefix)
	if err != nil {
		log.Fatal("failed to create test dir", err, "path", path)
	}
	// log.Info("test dir", "path", path)
	cleanup = func() {
		Purge(path)
	}
	return
}

// Removes the given directory and all of its content.
func Purge(path string) {
	// log.Info("purging test dir", "path", path)
	// util.PrintDirectoryTree(path)
	err := os.RemoveAll(path)
	if err != nil {
		log.Warn("failed to remove test directory", "path", path)
	}
}

// CopyDir copies the content of the 'source' directory into the 'destination'
// directory.
// If an 'accept' function is passed, only files whose path relative to the
// source directory is accepted by 'accept' are copies.
func CopyDir(source, destination string, accept ...func(relPath string) bool) error {

	var filter func(relPath string) bool
	if len(accept) > 0 && accept[0] != nil {
		filter = accept[0]
	}

	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath := strings.Replace(path, source, "", 1)
		if relPath == "" {
			return nil
		}
		if strings.HasPrefix(relPath, "/") {
			relPath = relPath[1:]
		}
		if filter != nil && !filter(relPath) {
			return nil
		}

		if info.IsDir() {
			return os.Mkdir(filepath.Join(destination, relPath), 0755)
		} else {
			var data, ex = ioutil.ReadFile(filepath.Join(source, relPath))
			if ex != nil {
				return ex
			}
			return ioutil.WriteFile(filepath.Join(destination, relPath), data, 0777)
		}
	})
	return err
}
