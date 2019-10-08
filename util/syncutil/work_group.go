package syncutil

import (
	"runtime"
	"sync"

	"eluvio/errors"
)

const queueSize = 1024

type Workfn func() error

type WorkGroup struct {
	wg      *sync.WaitGroup
	ch      chan Workfn
	name    string
	workers []*worker
}

func NewWorkGroup(name string, workerCount int, failFast bool) *WorkGroup {
	return newWorkGroup(name, workerCount, queueSize, failFast)
}

func newWorkGroup(name string, workerCount, qs int, failFast bool) *WorkGroup {
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}
	res := &WorkGroup{
		wg:      &sync.WaitGroup{},
		ch:      make(chan Workfn, qs),
		name:    name,
		workers: make([]*worker, 0, workerCount),
	}
	var errorCb func()
	if failFast {
		errorCb = res.closeChan
	}
	for i := 0; i < workerCount; i++ {
		w := newWorker(res.wg, res.ch, errorCb)
		res.workers = append(res.workers, w)
		go w.run()
	}
	return res
}

func (w *WorkGroup) Add(fns ...Workfn) (err error) {
	if len(fns) == 0 {
		return
	}
	// handle case where errors would lead to ch closed
	defer func() {
		if ex := recover(); ex != nil {
			if exx, ok := ex.(error); ok {
				err = exx
				return
			}
			err = errors.E("Add", errors.K.Invalid, "cause", ex)
		}
	}()

	for _, f := range fns {
		w.ch <- f
	}
	return
}

func (w *WorkGroup) closeChan() {
	defer func() {
		if ex := recover(); ex != nil {
		}
	}()
	close(w.ch)
}

func (w *WorkGroup) CloseWait() error {
	w.closeChan()
	w.wg.Wait()
	errs := make([]error, 0)
	for _, wr := range w.workers {
		if len(wr.errs) > 0 {
			errs = append(errs, wr.errs...)
		}
	}
	if len(errs) > 1 {
		// note: output is not really nice ..
		return errors.E(w.name, errors.K.Invalid, errs[0], "other_causes", errs[1:])
	} else if len(errs) == 1 {
		return errs[0]
	}
	return nil
}

type worker struct {
	wg    *sync.WaitGroup
	ch    chan Workfn
	errCb func()
	errs  []error
}

func newWorker(wg *sync.WaitGroup, c chan Workfn, errCb func()) *worker {
	wg.Add(1)
	return &worker{
		wg:    wg,
		ch:    c,
		errCb: errCb,
		errs:  make([]error, 0),
	}
}

func (w *worker) run() {
	for f := range w.ch {
		err := f()
		if err != nil {
			w.errs = append(w.errs, err)
			if w.errCb != nil {
				go w.errCb()
			}
		}
	}
	w.wg.Done()
}

/** PENDING(GIL): REMOVE
type SimpleWorkGroup struct {
	wg   *sync.WaitGroup
	ch   chan func()
	name string
}

func NewSimpleWorkGroup(name string, workerCount int) *SimpleWorkGroup {
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}
	res := &SimpleWorkGroup{
		wg:   &sync.WaitGroup{},
		ch:   make(chan func(), queueSize),
		name: name,
	}
	for i := 0; i < workerCount; i++ {
		res.wg.Add(1)
		go func() {
			for f := range res.ch {
				f()
			}
			res.wg.Done()
		}()
	}
	return res
}

func (w *SimpleWorkGroup) Add(fns ...func()) (err error) {
	if len(fns) == 0 {
		return
	}
	// handle case where ch is closed
	defer func() {
		if ex := recover(); ex != nil {
			if exx, ok := ex.(error); ok {
				err = exx
				return
			}
			err = errors.E("Add", errors.K.Invalid, "cause", ex)
		}
	}()

	for _, f := range fns {
		w.ch <- f
	}
	return
}

func (w *SimpleWorkGroup) CloseWait() {
	close(w.ch)
	w.wg.Wait()
}

*/
