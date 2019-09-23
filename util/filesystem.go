package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ExecutablePath returns the file path of the executable of the current go
// process.
func ExecutablePath() string {
	ex, err := os.Executable()
	if err != nil {
		wd, err := os.Getwd()
		if err != nil {
			return "."
		}
		return wd
	}
	return filepath.Dir(ex)
}

func PrintDirectoryTree(dir string) {
	fmt.Printf("Content of directory %s\n%s", dir, DirectoryTree(dir))
}

func DirectoryTree(dir string) string {
	var sb strings.Builder
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		p, _ := filepath.Rel(dir, path)
		indent := strings.Repeat("  ", strings.Count(p, "/"))
		ftype := "f"
		if info.IsDir() {
			ftype = "d"
		}
		sb.WriteString(fmt.Sprintf("%s%-"+strconv.Itoa(60-len(indent))+"s %s %.d\n", indent, info.Name(), ftype, info.Size()))
		return nil
	})
	return sb.String()
}

func FileExists(name string) bool {
	_, err := os.Stat(name)
	return !os.IsNotExist(err)
}
