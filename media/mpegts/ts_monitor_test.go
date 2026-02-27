package mpegts_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/eluv-io/common-go/media/mpegts"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

func TestStreamMonitor(t *testing.T) {
	t.Skip("skipping test - for now just manual inspection of logs")

	// PENDING(LUK): add actual tests for detecting stream stalls.

	now := utc.UnixMilli(0)
	defer utc.MockNowFn(func() utc.UTC { return now })()

	log := elog.Get("/eluvio/node/srtpub/mpegts")
	log.SetLevel("debug")

	log.Debug("starting test")

	m := mpegts.NewTsMonitor()
	m.Start("test-stream")

	go func() {
		time.Sleep(100 * time.Millisecond)
		m.Stop()
	}()

	for i := 0; i < 500000; i++ {
		if i%10000 == 0 {
			m.SignalPart(fmt.Sprintf("hqt_%3d", i))
		}
		m.SignalPacket(188)
		now = now.Add(time.Millisecond)
		// time.Sleep(time.Millisecond) // also give the monitor a chance to run
	}

	log.Debug("test done")
	time.Sleep(100 * time.Millisecond)
}
