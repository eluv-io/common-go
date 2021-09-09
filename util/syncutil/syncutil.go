package syncutil

import (
	"github.com/qluvio/content-fabric/errors"
	"sync"
	"time"

	elog "github.com/qluvio/content-fabric/log"
)

var log = elog.Get("/eluvio/util/syncutil")

// AttemptErrorChannelSend sends a channel message without blocking
func AttemptErrorChannelSend(channel chan<- error, message error,
	attempts int, interval time.Duration) (err error) {

	if channel == nil {
		return
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		select {
		case channel <- message:
			return
		default:
			time.Sleep(interval)
		}
	}
	return errors.E("AttemptErrorChannelSend", errors.K.IO,
		"reason", "timed out", "attempts", attempts, "interval", interval)
}

// AttemptStringChannelSend sends a channel message without blocking
func AttemptStringChannelSend(channel chan<- string, message string,
	attempts int, interval time.Duration) (err error) {

	if channel == nil {
		return
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		select {
		case channel <- message:
			return
		default:
			time.Sleep(interval)
		}
	}
	return errors.E("AttemptStringChannelSend", errors.K.IO,
		"reason", "timed out", "attempts", attempts, "interval", interval)
}

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
