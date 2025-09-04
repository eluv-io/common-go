package ioutil_test

import (
	"bytes"
	"fmt"
	stdioutil "io/ioutil"
	"testing"

	"github.com/eluv-io/common-go/util/ioutil"

	"github.com/stretchr/testify/require"
)

func Example() {
	r := ioutil.NewRepeatingReader([]byte("1234567890"), 24)
	res, err := stdioutil.ReadAll(r)
	fmt.Printf("bytes read: %s\n", string(res))
	fmt.Printf("error     : %v\n", err)

	// Output:
	//
	// bytes read: 123456789012345678901234
	// error     : <nil>
}

func TestRepeatingReader(t *testing.T) {
	buf := []byte("0123456789")
	var fbuf []byte
	for i := 0; i < 10; i++ {
		fbuf = append(fbuf, buf...)
	}

	tests := []struct {
		len int64
		buf int
	}{
		// {len: 0, buf: 10},
		{len: 1, buf: 10},
		{len: 10, buf: 1},
		{len: 10, buf: 5},
		{len: 10, buf: 9},
		{len: 10, buf: 10},
		{len: 10, buf: 11},
		{len: 12, buf: 9},
		{len: 12, buf: 10},
		{len: 12, buf: 11},
		{len: 44, buf: 9},
		{len: 44, buf: 10},
		{len: 44, buf: 11},
		{len: 44, buf: 12},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("len-%d-buflen-%d", tt.len, tt.buf), func(t *testing.T) {
			res := &bytes.Buffer{}
			r := ioutil.NewRepeatingReader(buf, tt.len)
			copyBuf := make([]byte, tt.buf)
			n, err := ioutil.CopyBuffer(res, r, copyBuf)
			require.NoError(t, err)
			require.EqualValues(t, tt.len, n)
			require.EqualValues(t, fbuf[:tt.len], res.Bytes())
		})
	}

}
