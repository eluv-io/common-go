package testutil

import (
	"io/ioutil"
	"os"

	"github.com/qluvio/content-fabric/log"
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
