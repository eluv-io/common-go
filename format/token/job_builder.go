package token

import (
	"strconv"

	"github.com/eluv-io/common-go/format/id"
)

type JobBuilder struct {
	token *Token
}

func NewJobBuilder(aid id.ID, index int) *JobBuilder {
	return &JobBuilder{
		token: &Token{
			AllocationID: aid,
			Bytes:        []byte(strconv.Itoa(index)),
		},
	}
}

func (b *JobBuilder) WithNodeID(nid id.ID) {
	b.token.NID = nid
}

func (b *JobBuilder) Build() (*Token, error) {
	err := b.token.Validate()
	if err != nil {
		return nil, err
	}
	return b.token, nil
}
