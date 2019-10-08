package syncutil

import (
	"sync"
	"time"

	elog "eluvio/log"
)

var log = elog.Get("/eluvio/util/syncutil")

// WaitTimeout waits for the waitgroup for the specified max timeout.
// Returns true if waiting timed out.
//
// NOTE: upon timeout, any go routines the wait group is waiting on are NOT
// INTERRUPTED in any way - they continue to run
func WaitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	start := time.Now()
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	timer := time.NewTimer(timeout)
	select {
	case <-c:
		if log.IsDebug() {
			log.Debug("wait finished before timeout", "timeout", timeout, "actual_duration", time.Now().Sub(start))
		}
		if !timer.Stop() {
			<-timer.C
		}
		return false // completed normally
	case <-timer.C:
		if log.IsInfo() {
			log.Info("wait timed out!", "timeout", timeout, "actual_duration", time.Now().Sub(start))
		}
		return true // timed out
	}
}
