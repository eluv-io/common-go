package ioutil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/errors-go"
)

func TestFailingReader(t *testing.T) {
	bb := []byte("abcdefghijklmnopqrstuvwxyz0123456789")

	type testCase struct {
		failAt  int64
		readLen int
		wantErr error
	}
	for i, tc := range []*testCase{
		{failAt: 10, readLen: 1},
		{failAt: 10, readLen: 5},
		{failAt: 10, readLen: 10},
		{failAt: 10, readLen: 100},
		{failAt: 10, readLen: 5, wantErr: errors.Str("for test")},
	} {
		br := bytes.NewReader(bb)
		fr := NewFailingReader(br, tc.failAt, tc.wantErr)
		var res []byte
		var err error
		for {
			p := make([]byte, tc.readLen)
			n := 0
			n, err = fr.Read(p)
			res = append(res, p[:n]...)
			if err != nil {
				break
			}
		}
		if tc.wantErr != nil {
			require.Equal(t, tc.wantErr, err, "case: %d, err: %v", i, err)
		}
		require.Equal(t, bb[:tc.failAt], res, "case: %d, err: %v", i, err)
		require.Equal(t, tc.failAt, fr.bytesCount, "case: %d, err: %v", i, err)
		require.Equal(t, len(bb)-int(tc.failAt), br.Len(), "case: %d, err: %v", i, err)
	}
}
