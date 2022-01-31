package ioutil_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eluv-io/common-go/util/ioutil"

	"github.com/stretchr/testify/require"
)

func TestValidatingWriter(t *testing.T) {
	var tests = []struct {
		name       string
		src        string
		ref        string
		success    bool
		errMatches string
	}{
		{
			name:    "t1",
			src:     "abcdefghijklmnopqrstuvwxyz",
			ref:     "abcdefghijklmnopqrstuvwxyz",
			success: true,
		},
		{
			name:    "t2",
			src:     "abcdefghijklmnopqrstuvwxyz",
			ref:     "abcdefghijklmnopqrstuvwxyz012345678",
			success: true,
		},
		{
			name:       "t3",
			src:        "abcdefghijklmnopqrstuvwxyz012345678",
			ref:        "abcdefghijklmnopqrstuvwxyz",
			success:    false,
			errMatches: `off \[26\].*unexpected EOF`,
		},
		{
			name:       "t4",
			src:        "abcdefghijklmn012345678",
			ref:        "abcdefghijklmnopqrstuvwxyz",
			success:    false,
			errMatches: `reason \[bytes differ\] off \[14\]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, 7)
			src := strings.NewReader(tt.src)
			w := ioutil.NewValidatingWriter(strings.NewReader(tt.ref))
			n, err := ioutil.CopyBuffer(w, src, buf)
			if tt.success {
				require.NoError(t, err)
				require.Equal(t, len(tt.src), n)
			} else {
				fmt.Println(err)
				require.Error(t, err)
				require.Regexp(t, tt.errMatches, err.Error())
			}
		})
	}

}
