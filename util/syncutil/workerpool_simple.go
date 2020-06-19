package syncutil

import (
	"sync"
	"time"

	"go.uber.org/atomic"
)

// NewSimpleWorkerPool creates a pool of worker goroutines.
func NewSimpleWorkerPool(maxWorkers int, idleTimeout time.Duration) *SimpleWorkerPool {
	// There must be at least one worker.
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	pool := &SimpleWorkerPool{
		maxWorkers:  int32(maxWorkers),
		idleTimeout: idleTimeout,
		workerQueue: make(chan func()),
	}

	return pool
}

// SimpleWorkerPool is a pool of worker go-routines that execute tasks with a
// controlled degree of parallelism by limiting the maximum number of workers
// that are running concurrently.
//
// Workers are created as needed, limited to maxWorkers that will execute tasks
// concurrently.  Idle workers are shutdown if not needed for idleTimeout
// duration.
//
// Submitting tasks to the pool when all workers are busy will block.
//
// There is no Stop() or Destroy() method, since the pool cleans up idle workers
// automatically if no new tasks are submitted.
type SimpleWorkerPool struct {
	maxWorkers  int32
	idleTimeout time.Duration
	workerQueue chan func()
	stopChan    chan func() // only used for stopping the worker pool
	stopped     atomic.Bool
	mutex       sync.Mutex
	workerCount atomic.Int32
	workerIdle  atomic.Int32
}

// Submit passes a task function to a worker for execution.
//
// Any external values needed by the task function must be captured in a
// closure.  Any return values should be returned over a channel that is
// captured in the task function closure.
//
// If a worker is available, the task is handed over and Submit returns
// immediately. Otherwise, a new worker is created and passed the task. The call
// blocks if the maxWorkers limit has already been reached until one of the
// existing workers becomes available.
func (p *SimpleWorkerPool) Submit(task func()) {
	if task == nil {
		return
	}

	select {
	case p.workerQueue <- task:
	default:
		// no workers ready...
		p.tryCreateWorker()
		// submit again - will block until a worker is ready
		p.workerQueue <- task
	}
}

// SubmitWait submits the given task function and waits for its execution to
// complete.
func (p *SimpleWorkerPool) SubmitWait(task func()) {
	if task == nil {
		return
	}

	doneChan := make(chan struct{})
	p.Submit(func() {
		task()
		close(doneChan)
	})
	<-doneChan
}

// tryCreateWorker creates a new worker unless the maximum number of workers has
// already been reached.
func (p *SimpleWorkerPool) tryCreateWorker() {
	if p.workerCount.Inc() > p.maxWorkers {
		p.workerCount.Dec()
		return
	}
	p.startWorker()
}

// startWorker starts a goroutine that executes tasks given by the dispatcher.
func (p *SimpleWorkerPool) startWorker() {
	p.workerIdle.Inc()

	go func() {
		defer p.workerCount.Dec()
		defer p.workerIdle.Dec()
		timer := time.NewTimer(p.idleTimeout)

		for {
			select {
			case task := <-p.workerQueue:
				if task == nil {
					// stop signal -> stop worker
					return
				}

				// stop timer - no need to let it run and trigger
				if !timer.Stop() {
					<-timer.C
				}

				// run the task
				p.workerIdle.Dec()
				task()
				p.workerIdle.Inc()

				// reset the timer (already stopped above)
				timer.Reset(p.idleTimeout)

			case <-timer.C:
				// idle timeout -> stop worker
				return
			}
		}
	}()
}
