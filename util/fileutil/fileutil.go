package fileutil

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/djherbis/times"

	"github.com/eluv-io/errors-go"

	"github.com/eluv-io/common-go/util/byteutil"
	"github.com/eluv-io/common-go/util/stringutil"
)

// CheckAccessTimeEnabled checks whether access_time is enabled in the filesystem at the given path.
func CheckAccessTimeEnabled(path string) (bool, error) {
	e := errors.Template("CheckAccessTimeEnabled", errors.K.IO, "path", path)
	atn := filepath.Join(path, "atc"+stringutil.RandomString(8))
	var ati os.FileInfo
	atf, err := os.Create(atn)
	if err == nil {
		ati, err = atf.Stat()
		if err == nil {
			_, err = atf.Write(byteutil.RandomBytes(1024))
			if err == nil {
				err = atf.Close()
			}
		}
	}
	defer func(atf *os.File) {
		if atf != nil {
			_ = atf.Close()
			_ = os.Remove(atn)
		}
	}(atf)
	if err != nil {
		return false, e(err)
	}
	if ati.Sys() == nil {
		return false, e(errors.K.NotImplemented, "reason", "system stats not available", "path", atn)
	}
	at1 := times.Get(ati).AccessTime()
	time.Sleep(time.Millisecond * 5)
	atf, err = os.Open(atn)
	if err == nil {
		_, err = io.ReadAll(atf)
		if err == nil {
			err = atf.Close()
		}
	}
	defer func(atf *os.File) {
		if atf != nil {
			_ = atf.Close()
		}
	}(atf)
	if err != nil {
		return false, e(err)
	}
	ati, err = os.Stat(atn)
	if err != nil {
		return false, e(err)
	}
	at2 := times.Get(ati).AccessTime()
	return !at1.Equal(at2), nil
}
