package syncutil_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/eluv-io/common-go/util/syncutil"
)

func TestPromise(t *testing.T) {
	p := syncutil.NewPromise()

	data := "The Result"
	go func() {
		time.Sleep(time.Second)
		p.Resolve(data, nil)
	}()

	wg := sync.WaitGroup{}

	get := func() {
		val, err := p.Get()
		fmt.Println("got", val, err)
		require.NoError(t, err)
		require.Equal(t, data, val)
		wg.Done()
	}

	wg.Add(3)
	go get()
	go get()
	get()

	p.Await()

	wg.Wait()
}

func TestMarshaledFuture(t *testing.T) {
	p := syncutil.NewPromise()

	data := "The Result"
	go func() {
		time.Sleep(time.Second)
		p.Resolve(data, nil)
	}()

	future := syncutil.NewMarshaledFuture(p)

	jsn, err := json.Marshal(map[string]interface{}{
		"result": future,
	})
	require.NoError(t, err)
	require.Equal(t, `{"result":"The Result"}`, string(jsn))
}

func TestFuturesAwait(t *testing.T) {
	fs := syncutil.NewFutures()
	p := syncutil.NewPromise()
	fs.Add(p)

	count := atomic.NewInt64(0)
	go recursivePromises(fs, p, count)

	err := fs.Await()
	require.NoError(t, err)

	require.Equal(t, int64(10), count.Load())
}

func TestFuturesFutures(t *testing.T) {
	fs := syncutil.NewFutures()
	p := syncutil.NewPromise()
	fs.Add(p)

	count := atomic.NewInt64(0)
	go recursivePromises(fs, p, count)

	list := fs.Futures()
	require.Equal(t, int64(10), count.Load())
	require.Equal(t, 10, len(list))

	for i, future := range list {
		val, err := future.Get()
		require.NoError(t, err)
		require.Equal(t, int64(i+1), val)
	}
}

func recursivePromises(fs *syncutil.Futures, p syncutil.Promise, count *atomic.Int64) {
	time.Sleep(time.Millisecond * 10)
	i := count.Inc()

	if i < 10 {
		np := syncutil.NewPromise()
		fs.Add(np)
		go recursivePromises(fs, np, count)
	}

	p.Resolve(i, nil)
}
