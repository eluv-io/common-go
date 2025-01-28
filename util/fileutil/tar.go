package fileutil

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/eluv-io/errors-go"
	elog "github.com/eluv-io/log-go"
)

var log = elog.Get("/eluvio/util/fileutil")

// Extract extracts all files from a source tar.gz archive to the given destination directory. The destination directory
// and any missing parents are created if they don't exist.
func Extract(source string, destination string) error {
	e := errors.Template("Extract", "source", source, "destination", destination)

	file, err := os.Open(source)
	if err != nil {
		return e(err)
	}
	defer log.Call(file.Close, "close source archive")

	err = os.MkdirAll(destination, 0755)
	if err != nil {
		return e(err)
	}

	gzipReader, err := gzip.NewReader(bufio.NewReader(file))
	if err != nil {
		return e(err)
	}
	defer log.Call(gzipReader.Close, "close gzipReader")

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return e(err)
		}

		fileInfo := header.FileInfo()
		dir := filepath.Join(destination, filepath.Dir(header.Name))
		filename := filepath.Join(dir, fileInfo.Name())

		err = os.MkdirAll(dir, 0755)
		if err != nil {
			return e(err)
		}

		err = func() error {
			file, err := os.Create(filename)
			if err != nil {
				return e(err)
			}
			defer func() {
				err2 := file.Close()
				if err == nil {
					err = err2
				}
			}()

			writer := bufio.NewWriter(file)

			_, err = io.Copy(writer, tarReader)
			if err != nil {
				return e(err)
			}

			err = writer.Flush()
			if err != nil {
				return e(err)
			}

			return nil
		}()

		if err != nil {
			return err
		}
	}

	return e.IfNotNil(err)
}

// Archive tars and gzips the content of the 'source' directory or file into the 'destination' archive path. Any parent
// directories of the archive path are created if they don't exist. If an 'accept' function is provided, only files
// accepted by the function are added to the archive.
func Archive(source, destination string, accept ...func(relPath string) bool) error {
	e := errors.Template("Archive", "source", source, "destination", destination)

	var filter func(relPath string) bool
	if len(accept) > 0 && accept[0] != nil {
		filter = accept[0]
	}

	err := os.MkdirAll(filepath.Dir(destination), 0775)
	if err != nil {
		return e(err)
	}

	file, err := os.Create(destination)
	if err != nil {
		return e(err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(destination)
		}
	}()

	gzipWriter := gzip.NewWriter(file)
	defer log.Call(gzipWriter.Close, "close gzipWriter")
	tarWriter := tar.NewWriter(gzipWriter)
	defer log.Call(tarWriter.Close, "close tarWriter")

	source = filepath.Clean(source)
	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath := path[len(source):]
		if relPath == "" {
			return nil
		}
		if strings.HasPrefix(relPath, "/") {
			relPath = relPath[1:]
		}
		if filter != nil && !filter(relPath) {
			return nil
		}
		if !info.IsDir() {
			return writeFileToTar(tarWriter, path, info, relPath)
		}
		return nil
	})

	return e.IfNotNil(err)
}

// writeFileToTar writes the given source file to the tar writer at the given archive path.
func writeFileToTar(tarWriter *tar.Writer, source string, sourceInfo os.FileInfo, archivePath string) error {
	e := errors.Template("writeFileToTar", "source", source, "archive_path", archivePath)

	file, err := os.Open(source)
	if err != nil {
		return e(err)
	}
	defer errors.Ignore(file.Close)

	symLinkTargetPath, err := filepath.EvalSymlinks(source)
	if err != nil {
		return e(err)
	}

	link := ""
	if symLinkTargetPath != source {
		link = symLinkTargetPath
	}

	header, err := tar.FileInfoHeader(sourceInfo, link)
	if err != nil {
		return e(err)
	}
	header.Name = archivePath

	err = tarWriter.WriteHeader(header)
	if err != nil {
		return e(err)
	}

	_, err = io.Copy(tarWriter, file)
	if err != nil {
		return e(err)
	}

	return nil
}
