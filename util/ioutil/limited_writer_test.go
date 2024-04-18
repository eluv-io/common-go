package ioutil

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/sliceutil"
)

func TestLimitedWriter(t *testing.T) {
	tests := []struct {
		data      [][]byte
		limit     int
		wantError bool
	}{
		{
			data:      [][]byte{[]byte("hello")},
			limit:     5,
			wantError: false,
		},
		{
			data:      [][]byte{[]byte("hello")},
			limit:     4,
			wantError: true,
		},
		{
			data:      [][]byte{[]byte("hello"), []byte("world")},
			limit:     10,
			wantError: false,
		},
		{
			data:      [][]byte{[]byte("hello"), []byte("world")},
			limit:     8,
			wantError: true,
		},
	}
	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var err error
			buf := &bytes.Buffer{}
			w := NewLimitedWriter(buf, test.limit)
			for _, bts := range test.data {
				_, err = w.Write(bts)
				if err != nil {
					if !test.wantError {
						require.NoError(t, err)
					}
					break
				}
			}
			if test.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				var all []byte
				for _, bts := range test.data {
					all = sliceutil.Append(bts, all, false)
				}
				require.Equal(t, all, buf.Bytes())
			}
		})
	}
}
