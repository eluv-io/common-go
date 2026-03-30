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

func TestParseQihot(t *testing.T) {
	tests := []struct {
		qihot   string
		wantQid types.QID
		wantHsh types.QHash
		wantTok types.QWriteToken
	}{
		{"", nil, nil, nil},
		{"abcd", nil, nil, nil},
		{"iq__48iLSSjzN3PRyzwWqDmG5Dx1zkfL", id.MustParse("iq__48iLSSjzN3PRyzwWqDmG5Dx1zkfL"), nil, nil},
		{
			"hq__EKjpzYq4vjPxchdoSm8fUSvK2y3PYVgLPdMWP8yqRRvu4rBnv3BY1BS7pdjVjfvvsasaTZA9qq",
			id.MustParse("iq__3MRbyPWE1EwEnPb2uNgVPHgF57Qj"),
			hash.MustParse("hq__EKjpzYq4vjPxchdoSm8fUSvK2y3PYVgLPdMWP8yqRRvu4rBnv3BY1BS7pdjVjfvvsasaTZA9qq"),
			nil,
		},
		{
			"tqw__8UmhDD9cZah58THfAYPf3Shj9hVzfwT51Cf4ZHKpayajzZRyMwCPiSpfS5yqRZfjkDjrtXuRmDa",
			id.MustParse("iq__99d4kp14eSDEP7HWfjU4W6qmqDw"),
			nil,
			token.MustParse("tqw__8UmhDD9cZah58THfAYPf3Shj9hVzfwT51Cf4ZHKpayajzZRyMwCPiSpfS5yqRZfjkDjrtXuRmDa"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.qihot, func(t *testing.T) {
			qid, hsh, tok := ParseQihot(tt.qihot)
			assert.Equal(t, tt.wantQid, qid)
			assert.Equal(t, tt.wantHsh, hsh)
			assert.Equal(t, tt.wantTok, tok)
		})
	}
}

func TestParseQhot(t *testing.T) {
	tests := []struct {
		qhot    string
		wantQid types.QID
		wantHsh types.QHash
		wantTok types.QWriteToken
	}{
		{"", nil, nil, nil},
		{"abcd", nil, nil, nil},
		{"iq__48iLSSjzN3PRyzwWqDmG5Dx1zkfL", nil, nil, nil},
		{
			"hq__EKjpzYq4vjPxchdoSm8fUSvK2y3PYVgLPdMWP8yqRRvu4rBnv3BY1BS7pdjVjfvvsasaTZA9qq",
			id.MustParse("iq__3MRbyPWE1EwEnPb2uNgVPHgF57Qj"),
			hash.MustParse("hq__EKjpzYq4vjPxchdoSm8fUSvK2y3PYVgLPdMWP8yqRRvu4rBnv3BY1BS7pdjVjfvvsasaTZA9qq"),
			nil,
		},
		{
			"tqw__8UmhDD9cZah58THfAYPf3Shj9hVzfwT51Cf4ZHKpayajzZRyMwCPiSpfS5yqRZfjkDjrtXuRmDa",
			id.MustParse("iq__99d4kp14eSDEP7HWfjU4W6qmqDw"),
			nil,
			token.MustParse("tqw__8UmhDD9cZah58THfAYPf3Shj9hVzfwT51Cf4ZHKpayajzZRyMwCPiSpfS5yqRZfjkDjrtXuRmDa"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.qhot, func(t *testing.T) {
			qid, hsh, tok := ParseQhot(tt.qhot)
			assert.Equal(t, tt.wantHsh, hsh)
			assert.Equal(t, tt.wantTok, tok)
			assert.Equal(t, tt.wantQid, qid, "%s", qid)
		})
	}
}
