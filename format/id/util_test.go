package id_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/id"
)

func TestExtract(t *testing.T) {
	tid := id.Generate(id.Tenant)
	qid := id.Generate(id.Q)
	lid := id.Generate(id.QLib)
	tqid := id.Compose(id.TQ, qid.Bytes(), tid.Bytes())
	tlid := id.Compose(id.TLib, lid.Bytes(), tid.Bytes())
	einv := id.Compose(id.TLib, lid.Bytes(), nil)
	einv2 := id.Decompose(id.MustParse("itl_13kpn8nqBySRQV"))

	fmt.Println(einv2.Explain())

	tests := []struct {
		target  id.Code
		ids     []string
		want    id.ID
		wantErr bool
	}{
		{id.Tenant, nil, nil, false},
		{id.Tenant, []string{""}, nil, false},
		{id.Tenant, []string{tid.String()}, tid, false},
		{id.Tenant, []string{"", qid.String(), lid.String(), tid.String()}, tid, false},
		{id.Tenant, []string{tqid.String()}, tid, false},
		{id.Tenant, []string{tlid.String()}, tid, false},
		{id.Tenant, []string{einv.String()}, id.NewID(id.Tenant, nil), false},
		{id.Tenant, []string{einv2.String()}, id.NewID(id.Tenant, nil), false},
		{id.Q, []string{tqid.String()}, tqid.ID(), false},
		{id.QLib, []string{tlid.String()}, tlid.ID(), false},
		{id.Tenant, []string{"no ID!"}, nil, true},
		{id.Tenant, []string{"", qid.String(), "no ID!"}, nil, true},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprint(tt.target, " ", strings.Join(tt.ids, "|")), func(t *testing.T) {
			got, err := id.Extract(tt.target, tt.ids...)
			require.Equal(t, tt.want, got, "case %d", i)
			if tt.wantErr {
				require.Error(t, err, "case %d", i)
			} else {
				require.NoError(t, err, "case %d", i)
			}
		})
	}
}
