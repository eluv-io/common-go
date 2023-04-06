package header

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHeader(t *testing.T) {
	tests := []struct {
		path      string
		wantPanic bool
	}{
		{
			path:      "/test-codec",
			wantPanic: false,
		},
		{
			path:      strings.Repeat("a", 126),
			wantPanic: false,
		},
		{
			path:      strings.Repeat("b", 127),
			wantPanic: true,
		},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			var panicReason interface{}
			func() {
				defer func() {
					panicReason = recover()
				}()
				hdr := New(test.path)
				require.Equal(t, test.path, hdr.String())
				require.Equal(t, test.path, hdr.Path())

				buf := &bytes.Buffer{}
				err := WriteHeader(buf, hdr)
				require.NoError(t, err)

				rhdr, err := ReadHeader(buf)
				require.NoError(t, err)

				require.Equal(t, hdr, rhdr)
			}()
			if test.wantPanic {
				require.Equal(t, ErrVarints.Error(), panicReason)
			} else {
				require.Empty(t, panicReason)
			}
		})
	}

}

func TestReadHeader(t *testing.T) {
	tests := []struct {
		bts      []byte
		wantPath string
		wantErr  error
	}{
		{
			bts:      []byte{6, '/', 'c', 'b', 'o', 'r', '\n'},
			wantPath: "/cbor",
			wantErr:  nil,
		},
		{
			bts:      append(append([]byte{127}, bytes.Repeat([]byte{'a'}, 126)...), '\n'),
			wantPath: strings.Repeat("a", 126),
			wantErr:  nil,
		},
		{
			bts:     []byte{3, '/', 'c', 'b', 'o', 'r', '\n'},
			wantErr: ErrHeaderInvalid,
		},
		{
			bts:     []byte{7, '/', 'c', 'b', 'o', 'r', '\n'},
			wantErr: io.ErrUnexpectedEOF,
		},
	}

	for _, test := range tests {
		name := test.wantPath
		if test.wantErr != nil {
			name = test.wantErr.Error()
		}
		t.Run(name, func(t *testing.T) {
			hdr, err := ReadHeader(bytes.NewReader(test.bts))
			if test.wantErr != nil {
				require.Equal(t, test.wantErr, err)
			} else {
				require.Equal(t, test.wantPath, hdr.Path())
			}
		})
	}
}
