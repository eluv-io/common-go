package syncutil_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/syncutil"
)

func TestWorkerPoolFairExecution(t *testing.T) {
	pool := syncutil.NewWorkerPool(1, 5*time.Second, 20)

	rdv := make(chan string)

	// block the single worker thread of the worker pool
	pool.NewTaskQueue(1).Submit(func() {
		rdv <- "start"
	})

	q1 := pool.NewTaskQueue()
	q2 := pool.NewTaskQueue()
	q3 := pool.NewTaskQueue()

	res := make(chan string)

	// for each queue, submit 10 tasks
	for idx, q := range []syncutil.TaskQueue{q1, q2, q3} {
		msg := fmt.Sprintf("q%d", idx+1)
		for i := 0; i < 10; i++ {
			q.Submit(func() {
				time.Sleep(2 * time.Millisecond)
				res <- msg
			})
		}
		// give the multiqueue some time to submit tasks to the underlying
		// worker pool
		time.Sleep(5 * time.Millisecond)
	}

	// unblock the first task
	<-rdv

	// expect that tasks from each queue alternate
	for i := 0; i < 10; i++ {
		require.Equal(t, "q1", <-res)
		require.Equal(t, "q2", <-res)
		require.Equal(t, "q3", <-res)
	}
}
