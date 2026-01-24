//go:build linux

package rtp

import (
	"golang.org/x/sys/unix"
)

func init() {
	// TEMPORARY FOR TESTING: Lock all memory pages to avoid page faults
	// This prevents OS from paging out memory which can cause latency spikes
	err := unix.Mlockall(unix.MCL_CURRENT | unix.MCL_FUTURE)
	if err != nil {
		log.Warn("failed to lock memory pages", "error", err, "note", "may need CAP_IPC_LOCK capability or ulimit -l unlimited")
	} else {
		log.Info("successfully locked all memory pages to prevent page faults")
	}
}
