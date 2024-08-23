package syncutil

import (
	"runtime"
	"time"

	"github.com/eluv-io/common-go/util/multiqueue"
)

// NewWorkerPool creates a new WorkerPool.
//
//   - maxWorkers:      the maximum number of worker goroutines that will be used
//     to execute tasks concurrently. If <=0 maxWokers is set to
//     runtime.NumPCU()
//   - idleTimeout:     the time to hold on to idle workers. After this duration,
//     idle workers are shutdown.
//   - defaultQueueCap: the default capacity to use when creating new
//     WorkerQueues. If <=0 the default queue cap is set to 16.
func NewWorkerPool(maxWorkers int, idleTimeout time.Duration, defaultQueueCap int) WorkerPool {
	if defaultQueueCap <= 0 {
		defaultQueueCap = 16
	}
	if maxWorkers <= 0 {
		maxWorkers = runtime.NumCPU()
	}

	dp := &workerPool{
		defaultQueueCap: defaultQueueCap,
		in:              multiqueue.New(),
		pool:            NewSimpleWorkerPool(maxWorkers, idleTimeout),
	}
	dp.start()

	return dp
}

// WorkerPool is a pool of worker goroutines that will process tasks submitted
// through multiple input queues (aka task queues).
//
// A task is a simple function without arguments or return values -
// most-commonly a closure that allows capturing the input arguments from the
// calling context. If return values are produced, they should be returned
// through a channel.
//
// The pool guarantees fairness between multiple task queues by employing a
// MultiQueue that services the different queues in a round-robin fashion.
type WorkerPool interface {
	NewTaskQueue(cap ...int) TaskQueue
}

type workerPool struct {
	defaultQueueCap int
	in              multiqueue.MultiQueue
	pool            *SimpleWorkerPool
}

// NewTaskQueue creates a new input queue for task submission. Submitting tasks
// to the task queue block when the queue reaches its capacity.
//
//   - cap: the optional capacity of the task queue. If not specified or <= 0,
//     the default task queue cap of the WorkerPool is used.
func (p *workerPool) NewTaskQueue(cap ...int) TaskQueue {
	c := p.defaultQueueCap
	if len(cap) > 0 && cap[0] > 0 {
		c = cap[0]
	}
	return &inputAdapter{p.in.NewInput(c)}
}

// TaskQueue is the interface for an input queue which is used to submit tasks
// to the worker pool.
type TaskQueue interface {
	// Submit enqueues a function for a worker in the worker pool to execute.
	// Any external values needed by the function must be captured in a closure.
	// Any return values should be sent back through over a channel that is also
	// captured in the function closure.
	Submit(func())
	// Close closes this input queue. Subsequent attempts to submit more tasks
	// will panic (just like sending to a closed channel). The queue will be
	// removed from the worker pool as soon as the last task has finished
	// executing.
	Close()
}

// start starts the go routine that pumps tasks from the MultiQueue to the
// worker pool.
func (p *workerPool) start() {
	go func() {
		for {
			res, closed := p.in.Pop()
			if closed {
				return
			}
			p.pool.Submit(res.(func()))
		}
	}()
}

type inputAdapter struct {
	multiqueue.Input
}

func (i *inputAdapter) Submit(f func()) {
	i.Input.Push(f)
}
