package syncutil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/eluv-io/common-go/util/timeutil"
)

func TestWaitCondition(t *testing.T) {
	res := "poufff"
	sleep := 10 * time.Millisecond
	timeout := 95 * time.Millisecond
	checkCount := atomic.NewInt32(0)

	tests := []struct {
		timeout         time.Duration
		condFn          func() (string, bool)
		opts            WaitOptions
		want            string
		wantErr         bool
		wantChecks      int32
		wantChecksDelta int32
	}{
		{
			timeout: timeout,
			condFn: func() (string, bool) {
				checkCount.Add(1)
				return res, false
			},
			opts: WaitOptions{
				Sleep: sleep,
			},
			want:            "",
			wantErr:         true,
			wantChecks:      10,
			wantChecksDelta: 1,
		},
		{
			timeout: timeout,
			condFn: func() (string, bool) {
				checkCount.Add(1)
				return res, true
			},
			opts: WaitOptions{
				Sleep: sleep,
			},
			want:       res,
			wantErr:    false,
			wantChecks: 1,
		},
		{
			timeout: timeout,
			condFn: func() (string, bool) {
				if checkCount.Add(1) > 3 {
					return res, true
				}
				return res, false
			},
			opts: WaitOptions{
				Sleep: sleep,
			},
			want:       res,
			wantErr:    false,
			wantChecks: 4,
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			checkCount.Store(0)
			watch := timeutil.StartWatch()
			got, err := WaitCondition(test.timeout, test.condFn, test.opts)
			duration := watch.Duration()
			if test.wantErr {
				require.Error(t, err)
				require.Empty(t, got)
				require.LessOrEqual(t, timeout, duration)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.want, got)
				require.GreaterOrEqual(t, timeout, duration)
			}
			require.Condition(t, func() (success bool) {
				c := checkCount.Load()
				return test.wantChecks <= c && c <= test.wantChecks+test.wantChecksDelta
			}, "expected: %d, actual: %d", test.wantChecks, checkCount.Load())
		})
	}
}

func TestWaitConditionWithChannel(t *testing.T) {
	var count int
	ch := make(chan struct{})
	watch := timeutil.StartWatch()

	go func() {
		time.Sleep(20 * time.Millisecond)
		ch <- struct{}{}
	}()

	got, err := WaitCondition(
		100*time.Millisecond,
		func() (int, bool) {
			count++
			return count, count > 1
		},
		WaitOptions{TriggerChan: ch})
	duration := watch.Duration()
	require.NoError(t, err)
	require.Equal(t, 2, got)
	require.InDelta(t, 20*time.Millisecond, duration, float64(5*time.Millisecond))
}
