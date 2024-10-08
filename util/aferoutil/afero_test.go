package aferoutil

import (
	"fmt"
	iofs "io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eluv-io/errors-go"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util"
	"github.com/eluv-io/common-go/util/testutil"
)

func TestMoveFile(t *testing.T) {
	dir, cleanup := testutil.TestDir("move_file")
	defer func() {
		if t.Failed() {
			util.PrintDirectoryTree(dir)
		}
		cleanup()
	}()

	srcDir := filepath.Join(dir, "src")
	require.NoError(t, os.Mkdir(srcDir, os.ModePerm))

	dstDir := filepath.Join(dir, "dst")

	srcData := []byte("test data")

	srcPaths := make([]string, 10)
	dstPaths := make([]string, 10)
	for i := range srcPaths {
		srcPaths[i] = createSrc(t, srcDir, i, srcData)
		dstPaths[i] = filepath.Join(dstDir, filepath.Base(srcPaths[i]))
	}

	osfs := afero.NewOsFs()
	noRenameFs := &NoRenameFs{Fs: osfs}

	type args struct {
		fs  afero.Fs
		src string
		dst string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "empty src",
			args: args{
				fs:  osfs,
				src: "",
				dst: "",
			},
			wantErr: true,
		},
		{
			name: "empty dst",
			args: args{
				fs:  osfs,
				src: srcPaths[0],
				dst: "",
			},
			wantErr: true,
		},
		{
			name: "src is a dir",
			args: args{
				fs:  osfs,
				src: srcDir,
				dst: dstPaths[0],
			},
			wantErr: true,
		},
		{
			name: "dst is a dir",
			args: args{
				fs:  osfs,
				src: srcPaths[0],
				dst: srcDir,
			},
			wantErr: true,
		},
		{
			name: "success - dst dir does not exist",
			args: args{
				fs:  osfs,
				src: srcPaths[0],
				dst: dstPaths[0],
			},
			wantErr: false,
		},
		{
			name: "success - dst dir exists",
			args: args{
				fs:  osfs,
				src: srcPaths[1],
				dst: dstPaths[1],
			},
			wantErr: false,
		},
		{
			name: "success - no rename",
			args: args{
				fs:  noRenameFs,
				src: srcPaths[2],
				dst: dstPaths[2],
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MoveFile(tt.args.fs, tt.args.src, tt.args.dst)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				// src must be gone
				_, err := ioutil.ReadFile(tt.args.src)
				require.Error(t, err)
				// dst must be there
				data, err := ioutil.ReadFile(tt.args.dst)
				require.NoError(t, err)
				require.Equal(t, srcData, data)
			}
		})
	}
}

func createSrc(t *testing.T, dir string, index int, srcData []byte) string {
	srcPath := filepath.Join(dir, fmt.Sprintf("source-%02d.txt", index))
	err := ioutil.WriteFile(srcPath, srcData, os.ModePerm)
	require.NoError(t, err)
	return srcPath
}

type NoRenameFs struct {
	afero.Fs
}

func (f *NoRenameFs) Rename(string, string) error {
	return errors.E("rename", errors.K.Invalid)
}

func TestRecreateDir(t *testing.T) {
	tests := []struct {
		name    string
		old     []string
		new     []string
		exclude []string
		fn      func(string, bool) string
	}{
		{
			name:    "no_change",
			old:     []string{"a", "b", "c", "d/e", "f/g/h"},
			new:     []string{"a", "b", "c", "d/e", "f/g/h"},
			exclude: []string{},
			fn:      nil,
		},
		{
			name:    "replace_1_with_0",
			old:     []string{"a0", "b1", "c0", "d0/e1", "f1/g1/h1"},
			new:     []string{"a0", "b0", "c0", "d0/e0", "f0/g0/h0"},
			exclude: []string{},
			fn: func(p string, _ bool) string {
				return strings.ReplaceAll(p, "1", "0")
			},
		},
		{
			name:    "squash_to_top_level",
			old:     []string{"a", "b", "c", "d/e", "f/g/h"},
			new:     []string{"a", "b", "c", "de", "fgh"},
			exclude: []string{},
			fn: func(p string, _ bool) string {
				return strings.ReplaceAll(p, "/", "")
			},
		},
		{
			name:    "exclude_dir",
			old:     []string{"a0", "b1", "c0", "d0/e1", "f1/g1/h1"},
			new:     []string{"a0", "b0", "c0", "d0/e0", "f0/g0/h1"},
			exclude: []string{"f1/g1"},
			fn: func(p string, _ bool) string {
				return strings.ReplaceAll(p, "1", "0")
			},
		},
		{
			name:    "remove_1",
			old:     []string{"a0", "b1", "c0", "d0/e1", "f1/g1/h1"},
			new:     []string{"a0", "c0", "d0/"},
			exclude: []string{},
			fn: func(p string, _ bool) string {
				if strings.Contains(p, "1") {
					return ""
				}
				return p
			},
		},
		{
			name:    "empty_dirs",
			old:     []string{"a0", "b1", "c0", "d0/", "f1/g1/"},
			new:     []string{"a0", "b0", "c0", "d0/", "f0/g0/"},
			exclude: []string{},
			fn: func(p string, _ bool) string {
				return strings.ReplaceAll(p, "1", "0")
			},
		},
	}
	dir, cleanup := testutil.TestDir("recreate_dir")
	defer cleanup()
	fs := afero.NewOsFs()
	for _, test := range tests {
		path := filepath.Join(dir, test.name)
		// Create directories and files with 0750 perms
		err := fs.Mkdir(path, 0750)
		require.NoError(t, err)
		for _, fname := range test.old {
			fpath := filepath.Join(path, fname)
			if strings.HasSuffix(fname, "/") {
				err = fs.MkdirAll(fpath, 0750)
				require.NoError(t, err)
			} else {
				fdir := filepath.Dir(fpath)
				err = fs.MkdirAll(fdir, 0750)
				require.NoError(t, err)
				f, err := fs.OpenFile(fpath, os.O_RDWR|os.O_CREATE, 0750)
				require.NoError(t, err)
				err = f.Close()
				require.NoError(t, err)
				fi, err := fs.Stat(fdir)
				require.NoError(t, err)
				require.NotNil(t, fi)
				require.Equal(t, iofs.FileMode(0750), fi.Mode().Perm())
				fi, err = fs.Stat(fpath)
				require.NoError(t, err)
				require.NotNil(t, fi)
				require.Equal(t, iofs.FileMode(0750), fi.Mode().Perm())
			}
		}
		// Recreate directories
		n, err := RecreateDir(fs, path, test.fn, test.exclude...)
		require.NoError(t, err)
		require.Equal(t, len(test.new), n)
		// Check new directories with 0755 perms and existing files with 0750 perms
		for i, fname := range test.new {
			fpath := filepath.Join(path, fname)
			fdir := filepath.Dir(fpath)
			fi, err := fs.Stat(fdir)
			require.NoError(t, err)
			require.NotNil(t, fi)
			if slices.Contains(test.exclude, filepath.Dir(test.old[i])) {
				require.Equal(t, iofs.FileMode(0750), fi.Mode().Perm())
			} else {
				require.Equal(t, iofs.FileMode(0755), fi.Mode().Perm())
			}
			fi, err = fs.Stat(fpath)
			require.NoError(t, err)
			require.NotNil(t, fi)
			if strings.HasSuffix(fname, "/") {
				require.Equal(t, iofs.FileMode(0755), fi.Mode().Perm())
			} else {
				require.Equal(t, iofs.FileMode(0750), fi.Mode().Perm())
			}
		}
		_, err = fs.Stat(path + ".bak")
		require.True(t, os.IsNotExist(err))
	}
}
