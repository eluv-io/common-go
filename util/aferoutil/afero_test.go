package aferoutil

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/util"
	"github.com/qluvio/content-fabric/util/testutil"
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
