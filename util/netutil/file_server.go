package netutil

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/eluv-io/log-go"
)

// NewFileServer creates a lightweight file server.
func NewFileServer(fsRoot string, port int) *FileServer {
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
	}
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

func (fs *FileServer) Start() {
	fs.log.Info("starting file server", "root", fs.root, "port", fs.port)
	go func() { fs.log.Info("http server shutdown", fs.s.ListenAndServe()) }()
}

func (fs *FileServer) Stop() error {
	fs.log.Info("shutting down file server", "root", fs.root, "port", fs.port)
	c, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	return fs.s.Shutdown(c)
}
