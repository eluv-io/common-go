package netutil

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/eluv-io/common-go/util/testutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
)

// NewFileServer creates a lightweight file server, which is a very thin wrapper around go's
// `http.FileServer`. If port is zero, a free port is used.
//
// This implementation is probably not safe for production use, but good for testing.
func NewFileServer(fsRoot string, port int) (*FileServer, error) {
	stat, err := os.Stat(fsRoot)
	if err != nil {
		return nil, errors.E("NewFileServer", errors.K.IO, err)
	}
	if !stat.IsDir() {
		return nil, errors.E("NewFileServer", errors.K.Invalid,
			"reason", "fs_root is not a directory",
			"fs_root", fsRoot)
	}
	if port == 0 {
		port, err = testutil.FreePort()
		if err != nil {
			return nil, errors.E("NewFileServer", errors.K.IO, err)
		}
	}

	h := http.FileServer(http.Dir(fsRoot))
	s := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: h,
	}

	return &FileServer{
		port: port,
		root: fsRoot,
		s:    s,
		log:  log.Get("/eluvio/file_server"),
	}, nil
}

func (fs *FileServer) Port() int {
	return fs.port
}

type FileServer struct {
	root string
	port int

	s   *http.Server
	log *log.Log
}

func (fs *FileServer) SetLog(l *log.Log) {
	fs.log = l
}

func (fs *FileServer) doLog(msg string, args ...interface{}) {
	if fs.log == nil {
		return
	}
	fs.log.Info(msg, args...)
}

func (fs *FileServer) Start() {
	fs.doLog("starting file server", "root", fs.root, "port", fs.port)
	go func() { fs.doLog("file server shutdown", fs.s.ListenAndServe()) }()
}

func (fs *FileServer) Stop() error {
	fs.doLog("shutting down file server", "root", fs.root, "port", fs.port)
	c, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	return fs.s.Shutdown(c)
}
