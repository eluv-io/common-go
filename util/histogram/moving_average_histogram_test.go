package histogram

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCountToKeep(t *testing.T) {
	type testCase struct {
		durationPerHistogram time.Duration
		toCover              time.Duration
		wantCount            int
	}
	for i, tc := range []*testCase{
		{durationPerHistogram: time.Second * 1, toCover: time.Minute, wantCount: 61},
		{durationPerHistogram: time.Second * 5, toCover: time.Minute, wantCount: 13},
		{durationPerHistogram: time.Second * 10, toCover: time.Minute, wantCount: 7},
		{durationPerHistogram: time.Second * 20, toCover: time.Minute, wantCount: 4},
		{durationPerHistogram: time.Second * 29, toCover: time.Minute, wantCount: 4},
		{durationPerHistogram: time.Second * 30, toCover: time.Minute, wantCount: 3},
		{durationPerHistogram: time.Minute * 1, toCover: time.Minute, wantCount: 2},
		{durationPerHistogram: time.Minute + 1, toCover: time.Minute, wantCount: 2},
		{durationPerHistogram: time.Minute * 2, toCover: time.Minute, wantCount: 2},
		{durationPerHistogram: time.Minute * 3, toCover: time.Minute, wantCount: 2},
		{durationPerHistogram: time.Minute*3 + 1, toCover: time.Minute, wantCount: 2},
		{durationPerHistogram: time.Minute * 4, toCover: time.Minute, wantCount: 2},
		{durationPerHistogram: time.Second * 1, toCover: time.Hour, wantCount: 3601},
		{durationPerHistogram: time.Second * 5, toCover: time.Hour, wantCount: 721},
		{durationPerHistogram: time.Minute * 1, toCover: time.Hour, wantCount: 61},
		{durationPerHistogram: time.Minute * 5, toCover: time.Hour, wantCount: 13},
	} {
		ck := countToKeep(tc.durationPerHistogram, tc.toCover)
		require.Equal(t, tc.wantCount, ck, "failed case %d at %v / %v - expected %d, got %d", i, tc.toCover, tc.durationPerHistogram, tc.wantCount, ck)
	}
}
