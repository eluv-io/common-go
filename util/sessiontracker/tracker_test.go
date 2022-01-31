package sessiontracker_test

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/eluv-io/utc-go"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/sessiontracker"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func TestSessionTrackerBasic(t *testing.T) {
	now := utc.Now()
	defer utc.MockNowFn(func() utc.UTC {
		return now
	})()

	tracker := sessiontracker.New(duration.Spec(5 * time.Second))

	assertSessions := func(added, removed int64) {
		metrics := tracker.SessionMetrics()
		require.Equal(t, added, metrics.Added)
		require.Equal(t, removed, metrics.Removed)
		require.Equal(t, added-removed, metrics.Current)
		require.Equal(t, int(added-removed), tracker.Count())
	}

	assertSessions(0, 0)

	tracker.Update("id1")
	assertSessions(1, 0)

	now = now.Add(time.Second)
	assertSessions(1, 0)
	tracker.Update("id2")
	assertSessions(2, 0)

	tracker.Update("id1")
	tracker.Update("id2")
	assertSessions(2, 0)

	now = now.Add(time.Second)
	assertSessions(2, 0)
	tracker.Update("id3")
	assertSessions(3, 0)

	now = now.Add(time.Second)
	assertSessions(3, 0)
	tracker.Update("id4")
	assertSessions(4, 0)

	now = now.Add(time.Second)
	assertSessions(4, 0)
	tracker.Update("id5")
	assertSessions(5, 0)

	now = now.Add(time.Second)
	assertSessions(5, 0)
	tracker.Update("id6")
	assertSessions(6, 0)

	now = now.Add(time.Second)
	assertSessions(6, 2)
	tracker.Update("id7")
	assertSessions(7, 2)

	now = now.Add(time.Second)
	assertSessions(7, 3)
	tracker.Update("id8")
	assertSessions(8, 3)

	entries := tracker.List()
	require.Equal(t, "id4", entries[0].ID)
	require.Equal(t, now.Add(-4*time.Second), entries[0].LastUpdated)
	require.Equal(t, "id5", entries[1].ID)
	require.Equal(t, now.Add(-3*time.Second), entries[1].LastUpdated)
	require.Equal(t, "id6", entries[2].ID)
	require.Equal(t, now.Add(-2*time.Second), entries[2].LastUpdated)
	require.Equal(t, "id7", entries[3].ID)
	require.Equal(t, now.Add(-1*time.Second), entries[3].LastUpdated)
	require.Equal(t, "id8", entries[4].ID)
	require.Equal(t, now, entries[4].LastUpdated)
}

func TestSessionTrackerConcurrent(t *testing.T) {
	tracker := sessiontracker.New(duration.Spec(100 * time.Millisecond))
	termChan := make(chan bool)
	wg := &sync.WaitGroup{}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		id := fmt.Sprintf("id%d", i)
		go func() {
			for {
				select {
				case <-termChan:
					wg.Done()
					return
				default:
					tracker.Update(id)
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}

	time.Sleep(500 * time.Millisecond)
	close(termChan)
	wg.Wait()

	metrics := tracker.SessionMetrics()
	require.EqualValues(t, 10, metrics.Added)
	require.EqualValues(t, 0, metrics.Removed)
	require.EqualValues(t, 10, metrics.Current)

	time.Sleep(100 * time.Millisecond)
	metrics = tracker.SessionMetrics()
	require.EqualValues(t, 10, metrics.Added)
	require.EqualValues(t, 10, metrics.Removed)
	require.EqualValues(t, 0, metrics.Current)

	fmt.Println("sessions", jsonutil.Stringer(metrics))
	fmt.Println("metrics ", jsonutil.Stringer(tracker.Metrics()))
}

func TestSessionTrackerConcurrent2(t *testing.T) {
	tracker := sessiontracker.New(duration.Spec(10 * time.Millisecond))
	termChan := make(chan bool)
	wg := &sync.WaitGroup{}

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		id := fmt.Sprintf("id%d", i)
		go func() {
			timer := time.NewTimer(1)
			for {
				select {
				case <-termChan:
					wg.Done()
					return
				default:
					tracker.Update(id)
				}
				sleep := time.Millisecond * time.Duration(rand.Int63n(5)+7)
				timer.Reset(sleep)
				select {
				case <-timer.C:
				case <-termChan:
				}
			}
		}()
	}

	time.Sleep(1 * time.Second)
	close(termChan)
	wg.Wait()

	metrics := tracker.SessionMetrics()
	fmt.Println("sessions", jsonutil.Stringer(metrics))
	require.LessOrEqual(t, int64(1000), metrics.Added)
	require.EqualValues(t, metrics.Current, metrics.Added-metrics.Removed)

	time.Sleep(10 * time.Millisecond)
	metrics = tracker.SessionMetrics()
	require.EqualValues(t, 0, metrics.Added-metrics.Removed, jsonutil.Stringer(tracker.Metrics()))
	require.EqualValues(t, 0, metrics.Current)

	fmt.Println("sessions", jsonutil.Stringer(metrics))
	fmt.Println("metrics ", jsonutil.Stringer(tracker.Metrics()))
}
