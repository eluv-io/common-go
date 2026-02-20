package testutil

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/errors-go"
)

const (
	ElvTestAssets = "ELV_TEST_ASSETS"
)

// AssetsPathT is like AssetsPath, but requires that it returns no error.
func AssetsPathT(t require.TestingT, dirDepth int) string {
	res, err := AssetsPath(dirDepth)
	require.NoError(t, err)
	return res
}

// AssetsPath returns the path to the test assets directory. dirDepth is the number of directories from the project
// root that the test runs in.
func AssetsPath(dirDepth int) (string, error) {
	e := errors.Template("AssetsPath", errors.K.Invalid.Default())
	assetsPath, _ := os.LookupEnv(ElvTestAssets)
	if assetsPath == "" {
		e = e.Add("hint", "set the "+ElvTestAssets+" environment variable to the path of the test assets directory")
		assetsPath = strings.Repeat("../", dirDepth+1) + "test-assets"
	} else {
		e = e.Add("note", "using "+ElvTestAssets+" environment variable")
	}
	var err error
	assetsPath, err = filepath.Abs(assetsPath)
	if err != nil {
		return "", e(err)
	}

	info, err := os.Stat(assetsPath)
	if err != nil {
		return "", e(err)
	}
	if !info.IsDir() {
		return "", e("reason", "not a directory", "assets_path", assetsPath)
	}
	return assetsPath, nil
}
