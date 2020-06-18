package syncutil

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const max = 20

func TestExample(t *testing.T) {
	wp := NewSimpleWorkerPool(2, 5*time.Second)
	requests := []string{"alpha", "beta", "gamma", "delta", "epsilon"}

	rspChan := make(chan string, len(requests))
	for _, r := range requests {
		r := r
		wp.Submit(func() {
			rspChan <- r
		})
	}

	rspSet := map[string]struct{}{}
	for i := 0; i < len(requests); i++ {
		rsp := <-rspChan
		rspSet[rsp] = struct{}{}
	}
	for _, req := range requests {
		if _, ok := rspSet[req]; !ok {
			t.Fatal("Missing expected values:", req)
		}
	}
}

func TestMinMaxWorkers(t *testing.T) {
	t.Parallel()

	wp := NewSimpleWorkerPool(0, 5*time.Second)
	if wp.maxWorkers != 1 {
		t.Fatal("should use at least one worker")
	}
}

func TestMaxWorkers(t *testing.T) {
	t.Parallel()
	wp := NewSimpleWorkerPool(max, 5*time.Second)

	started := make(chan struct{}, max)
	stop := make(chan struct{})

	// Start workers, and have them all wait on a channel before completing.
	for i := 0; i < max; i++ {
		wp.Submit(func() {
			started <- struct{}{}
			<-stop
		})
	}

	// wait until the're all started
	timeout := time.After(1 * time.Second)
	for startCount := 0; startCount < max; {
		select {
		case <-started:
			startCount++
		case <-timeout:
			require.Fail(t, "timed out waiting for workers to start")
		}
	}

	// submit another task on a separate go-routine ==> should block since all
	// workers busy
	rdv := make(chan string)
	go func() {
		rdv <- "started"
		wp.Submit(func() {
			time.Sleep(time.Millisecond)
			rdv <- "running"
		})
		rdv <- "submitted"
	}()

	require.Equal(t, "started", <-rdv)

	for i := 0; i < 3; i++ {
		time.Sleep(10 * time.Millisecond)
		select {
		case msg := <-rdv:
			require.Fail(t, "received unexpected message %s", msg)
		default:
		}
	}

	// we should be running with the max number of workers
	require.EqualValues(t, max, wp.workerCount.Load())

	// Release current workers.
	close(stop)

	require.Equal(t, "submitted", <-rdv)
	require.Equal(t, "running", <-rdv)
}

func TestReuseWorkers(t *testing.T) {
	t.Parallel()

	wp := NewSimpleWorkerPool(5, 5*time.Second)

	rdv := make(chan struct{})

	// Cause worker to be created, and available for reuse before next task.
	for i := 0; i < 10; i++ {
		wp.Submit(func() { <-rdv })
		rdv <- struct{}{}
		time.Sleep(10 * time.Millisecond)
		require.EqualValues(t, 1, wp.workerIdle.Load(), "worker not reused! i=%d", i)
	}
}

func TestWorkerTimeout(t *testing.T) {
	t.Parallel()

	wp := NewSimpleWorkerPool(max, time.Second)

	stop := make(chan struct{})
	rdv := make(chan string, max)
	// Cause workers to be created.  Workers wait on channel, keeping them busy
	// and causing the worker pool to create more.
	for i := 0; i < max; i++ {
		wp.Submit(func() {
			rdv <- "started"
			<-stop
		})
	}

	// Wait for tasks to start.
	for i := 0; i < max; i++ {
		require.Equal(t, "started", <-rdv)
	}

	// ensure no idle workers
	require.EqualValues(t, 0, wp.workerIdle.Load())
	require.EqualValues(t, max, wp.workerCount.Load())

	// Release workers.
	close(stop)

	// ensure all workers now idle
	time.Sleep(10 * time.Millisecond)
	require.EqualValues(t, max, wp.workerIdle.Load())

	// wait for workers to timeout
	time.Sleep(time.Second)
	require.EqualValues(t, 0, wp.workerIdle.Load())
	require.EqualValues(t, 0, wp.workerCount.Load())
}

func BenchmarkSimpleWorkerPool1(b *testing.B) {
	benchmarkExecWorkers(1, b)
}

func BenchmarkSimpleWorkerPool2(b *testing.B) {
	benchmarkExecWorkers(2, b)
}

func BenchmarkSimpleWorkerPool4(b *testing.B) {
	benchmarkExecWorkers(4, b)
}

func BenchmarkSimpleWorkerPool16(b *testing.B) {
	benchmarkExecWorkers(16, b)
}

func BenchmarkSimpleWorkerPool64(b *testing.B) {
	benchmarkExecWorkers(64, b)
}

func BenchmarkSimpleWorkerPool1024(b *testing.B) {
	benchmarkExecWorkers(1024, b)
}

func benchmarkExecWorkers(n int, b *testing.B) {
	wp := NewSimpleWorkerPool(n, 5*time.Millisecond)

	var allDone sync.WaitGroup
	allDone.Add(b.N * n)

	b.ResetTimer()

	// Start workers, and have them all wait on a channel before completing.
	for i := 0; i < b.N; i++ {
		for j := 0; j < n; j++ {
			wp.Submit(func() {
				//time.Sleep(100 * time.Microsecond)
				allDone.Done()
			})
		}
	}
	allDone.Wait()
}
