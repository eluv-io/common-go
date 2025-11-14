package format

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/format/token"
	"github.com/eluv-io/common-go/format/types"
)

func TestExtractQID(t *testing.T) {
	tests := []struct {
		qhit string
		want types.QID
	}{
		{"", nil},
		{"abcd", nil},
		{"iq__48iLSSjzN3PRyzwWqDmG5Dx1zkfL", id.MustParse("iq__48iLSSjzN3PRyzwWqDmG5Dx1zkfL")},
		{"hq__EKjpzYq4vjPxchdoSm8fUSvK2y3PYVgLPdMWP8yqRRvu4rBnv3BY1BS7pdjVjfvvsasaTZA9qq", hash.MustParse("hq__EKjpzYq4vjPxchdoSm8fUSvK2y3PYVgLPdMWP8yqRRvu4rBnv3BY1BS7pdjVjfvvsasaTZA9qq").ID},
		{"tqw__8UmhDD9cZah58THfAYPf3Shj9hVzfwT51Cf4ZHKpayajzZRyMwCPiSpfS5yqRZfjkDjrtXuRmDa", token.MustParse("tqw__8UmhDD9cZah58THfAYPf3Shj9hVzfwT51Cf4ZHKpayajzZRyMwCPiSpfS5yqRZfjkDjrtXuRmDa").QID},
	}
	for _, tt := range tests {
		t.Run(tt.qhit, func(t *testing.T) {
			assert.Equal(t, tt.want, ExtractQID(tt.qhit))
		})
	}
}
