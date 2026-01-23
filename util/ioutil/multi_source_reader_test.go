package ioutil_test

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/byteutil"
	"github.com/eluv-io/common-go/util/ioutil"
	"github.com/eluv-io/errors-go"
)

func TestMultiSourceReader(t *testing.T) {
	count := 10
	for n := 0; n < count; n++ {
		r := ioutil.NewMultiSourceReader(nil)
		buf := byteutil.RandomBytes(128 * 1024)
		for i := 0; i < 4; i++ {
			r.Add(newTestSourceReader(buf, i == 0))
		}
		b, err := io.ReadAll(r)
		require.NoError(t, err)
		require.Equal(t, buf, b)
		err = r.Close()
		require.NoError(t, err)
		fmt.Printf("%d of %d done\n", n+1, count)
	}

	r := ioutil.NewMultiSourceReader(nil)
	buf := byteutil.RandomBytes(128 * 1024)
	for i := 0; i < 4; i++ {
		r.Add(newTestSourceReader(buf, true))
	}
	b, err := io.ReadAll(r)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed early")
	require.Equal(t, buf[:len(b)], b)
	err = r.Close()
	require.NoError(t, err)
}

func newTestSourceReader(buf []byte, fail bool) io.ReadCloser {
	n := 0
	if fail {
		n = len(buf)/2 + rand.Intn(len(buf)/2) - 1
	}
	return &testSourceReader{r: bytes.NewBuffer(buf), fail: n}
}

type testSourceReader struct {
	r      io.Reader
	off    int
	fail   int
	failed bool
	closed bool
}

func (r *testSourceReader) Read(p []byte) (int, error) {
	if r.failed {
		return 0, errors.E("read after fail")
	} else if r.closed {
		return 0, errors.E("read after close")
	}
	time.Sleep(time.Millisecond * time.Duration(rand.Intn(100)))
	if r.fail > 0 && r.off >= r.fail {
		r.failed = true
		return 0, errors.E("failed early")
	}
	n, err := r.r.Read(p)
	r.off += n
	return n, err
}

func (r *testSourceReader) Close() error {
	if r.closed {
		return errors.E("close after close")
	}
	r.closed = true
	return nil
}
