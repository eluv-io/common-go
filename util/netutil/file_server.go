package netutil

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/eluv-io/log-go"
)

// NewFileServer creates a lightweight file server. This is only suitable for tests, as
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

func (fs *FileServer) Start() error {
	go func() { log.Info("http server shutdown", fs.s.ListenAndServe()) }()
	return nil
}

func (fs *FileServer) Stop() error {
	c, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	return fs.s.Shutdown(c)
}
