package timeutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/format/utc"
)

func TestPeriodicInitialCall(t *testing.T) {
	p := NewPeriodic(time.Second)

	count := 0
	called := p.Do(func() {
		count++
	})

	require.True(t, called)
	require.Equal(t, 1, count)
}

func TestPeriodic(t *testing.T) {
	now := utc.Now()
	defer utc.MockNowFn(func() utc.UTC { return now })()

	tests := []struct {
		name          string
		testDuration  time.Duration
		sleep         time.Duration
		expectedCalls int
	}{
		{"hig frequency", 90 * time.Millisecond, 1 * time.Millisecond, 5},
		{"low frequency", 200 * time.Millisecond, 60 * time.Millisecond, 4},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := NewPeriodic(20 * time.Millisecond)
			count := 0
			calledCount := 0
			watch := StartWatch()

			for watch.Duration() < test.testDuration {
				called := p.Do(func() {
					count++
				})
				if called {
					calledCount++
				}
				now = now.Add(test.sleep)
			}

			require.Equal(t, test.expectedCalls, count)
			require.Equal(t, test.expectedCalls, calledCount)
		})
	}
}
