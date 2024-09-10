package byterange_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eluv-io/common-go/util/httputil/byterange"
)

func TestParseBytesRange(t *testing.T) {
	tests := []struct {
		bytes      string
		wantOffset int64
		wantSize   int64
		wantErr    bool
	}{
		{bytes: "0-99", wantOffset: 0, wantSize: 100, wantErr: false},
		{bytes: "100-199", wantOffset: 100, wantSize: 100, wantErr: false},
		{bytes: "100-", wantOffset: 100, wantSize: -1, wantErr: false},
		{bytes: "-100", wantOffset: -1, wantSize: 100, wantErr: false},
		{bytes: "-", wantOffset: 0, wantSize: -1, wantErr: false},
		{bytes: "", wantOffset: 0, wantSize: -1, wantErr: false},
		{bytes: "0", wantOffset: 0, wantSize: 0, wantErr: true},
		{bytes: "+400", wantOffset: 0, wantSize: 0, wantErr: true},
		{bytes: "abc-def", wantOffset: 0, wantSize: 0, wantErr: true},
		{bytes: "-0", wantOffset: -1, wantSize: 0, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.bytes, func(t *testing.T) {
			gotOffset, gotSize, err := byterange.Parse(tt.bytes)
			assert.Equal(t, tt.wantOffset, gotOffset)
			assert.Equal(t, tt.wantSize, gotSize)
			if tt.wantErr {
				assert.Error(t, err)
				fmt.Println(err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
