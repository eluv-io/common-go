package filterutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilters(t *testing.T) {
	tests := []struct {
		f    Filter
		want []bool
	}{
		{&BurstCount{}, []bool{false, false, false, false, false, false}},
		{&BurstCount{Deny: 4}, []bool{false, false, false, false, false, false}},
		{&BurstCount{Accept: 2, Deny: 1}, []bool{true, true, false, true, true, false, true}},
		{&BurstCount{Accept: 1}, []bool{true, true, true, true, true}},
		{&Manual{}, []bool{false, false, false, false, false, false}},
		{(&Manual{}).Set(true), []bool{true, true, true, true, true}},
	}

	for ti, test := range tests {
		for ci, want := range test.want {
			require.Equal(t, want, test.f.Filter(), "test %d call %d", ti, ci)
		}
	}
}
