package syncutil

import (
	"time"

	"github.com/eluv-io/common-go/util/numberutil"
	"github.com/eluv-io/common-go/util/timeutil"
	"github.com/eluv-io/errors-go"
)

type WaitOptions struct {
	// Sleep specifies the time to sleep between condition checks if no TriggerChan is specified. Defaults to 50ms.
	Sleep time.Duration

	// defaults to nil
	TriggerChan <-chan struct{}

	Log func(msg string, fields ...interface{})
}

// WaitCondition waits for the given condition to become true. Returns an error if the condition is still false after
// timeout. Uses the optional trigger channel to wait between condition checks or sleeps for the time specified in the
// wait options.
func WaitCondition[T any](timeout time.Duration, cond func() (T, bool), opts WaitOptions) (T, error) {
	if opts.Sleep == 0 {
		opts.Sleep = 50 * time.Millisecond
	}
	if opts.Log == nil {
		opts.Log = func(msg string, fields ...interface{}) {}
	}

	var zero T
	watch := timeutil.StartWatch()
	for {
		current, accept := cond()
		if accept {
			return current, nil
		}
		elapsed := watch.Duration()
		if timeout <= 0 {
			return zero, errors.E("condition not satisfied", "elapsed", elapsed, "timeout", timeout)
		}
		remaining := timeout - elapsed
		if remaining <= 0 {
			return zero, errors.E("condition not satisfied", "elapsed", elapsed, "timeout", timeout)
		}
		if opts.TriggerChan != nil {
			opts.Log("waiting for trigger or timer", "remaining", remaining)
			timer := time.NewTimer(remaining)
			select {
			case <-opts.TriggerChan:
				if !timer.Stop() {
					<-timer.C
				}
			case <-timer.C:
			}
		} else {
			sleep := numberutil.Min(opts.Sleep, remaining)
			opts.Log("sleeping", "duration", sleep)
			time.Sleep(sleep)
		}
	}
}
