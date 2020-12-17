package netutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindLocator(t *testing.T) {
	defLocs := []string{
		"http://node-1.contentfabric.net",
		"https://node-1.contentfabric.net",
	}
	tests := []struct {
		locs []string
		cand string
		want bool
	}{
		{defLocs, "http://node-1.contentfabric.net", true},
		{defLocs, "http://node-1.contentfabric.net:80", true},
		{defLocs, "http://node-1.contentfabric.net:80/", true},
		{defLocs, "https://node-1.contentfabric.net", true},
		{defLocs, "https://node-1.contentfabric.net:443", true},
		{defLocs, "https://node-1.contentfabric.net:443/", true},
		// not in list
		{defLocs, "http://node-2.contentfabric.net", false},
		{defLocs, "https://node-2.contentfabric.net:443", false},
		{defLocs, "https://node-1.contentfabric.net:888", false},
		// invalid candidates
		{defLocs, "node-1.contentfabric.net", false},
		{defLocs, "http://node-1.contentfabric.net:234234:2334", false},
	}
	for _, tt := range tests {
		t.Run(tt.cand, func(t *testing.T) {
			require.Equal(t, tt.want, FindLocator(tt.locs, tt.cand))
		})
	}
}

func TestNormalizeLocator(t *testing.T) {
	tests := []struct {
		loc     string
		want    string
		wantErr bool
	}{
		{"http://node-1.contentfabric.net", "http://node-1.contentfabric.net:80/", false},
		{"http://node-1.contentfabric.net:80", "http://node-1.contentfabric.net:80/", false},
		{"http://node-1.contentfabric.net:80/", "http://node-1.contentfabric.net:80/", false},
		{"http://node-1.contentfabric.net:1234", "http://node-1.contentfabric.net:1234/", false},
		{"http://node-1.contentfabric.net:1234/", "http://node-1.contentfabric.net:1234/", false},
		{"https://node-1.contentfabric.net", "https://node-1.contentfabric.net:443/", false},
		{"https://node-1.contentfabric.net:443", "https://node-1.contentfabric.net:443/", false},
		{"https://node-1.contentfabric.net:443/", "https://node-1.contentfabric.net:443/", false},
		{"https://node-1.contentfabric.net:1234", "https://node-1.contentfabric.net:1234/", false},
		{"https://node-1.contentfabric.net:1234/", "https://node-1.contentfabric.net:1234/", false},
	}
	for _, tt := range tests {
		t.Run(tt.loc, func(t *testing.T) {
			normalized, err := NormalizeLocator(tt.loc)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, normalized)
			}
		})
	}
}
