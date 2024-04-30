package syncutil

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/errors-go"
)

func TestWGSimple(t *testing.T) {
	var x uint64 = 0
	var exec uint64 = 0
	adder := func() error {
		atomic.AddUint64(&exec, 1)
		atomic.AddUint64(&x, 1)
		return nil
	}
	wg := newWorkGroup("adder", 2, 1, false)
	for i := 0; i < 10; i++ {
		err := wg.Add(adder)
		require.NoError(t, err)
	}
	err := wg.CloseWait()
	require.NoError(t, err)
	require.Equal(t, uint64(10), x)
	require.Equal(t, uint64(10), exec)

	x = 0
	exec = 0
	adder = func() error {
		atomic.AddUint64(&exec, 1)
		if atomic.LoadUint64(&x) == 3 {
			return errors.E("adder", errors.K.Invalid, "max", 3)
		}
		atomic.AddUint64(&x, 1)
		return nil
	}
	wg = newWorkGroup("adder", 2, 1, false)
	for i := 0; i < 10; i++ {
		err := wg.Add(adder)
		require.NoError(t, err)
	}
	err = wg.CloseWait()
	require.Error(t, err)
	//fmt.Println("err", err)
	require.Equal(t, uint64(3), x)
	require.Equal(t, uint64(10), exec)
}

func TestWGFailFast(t *testing.T) {
	var x uint64 = 0
	var exec uint64 = 0
	adder := func() error {
		atomic.AddUint64(&exec, 1)
		if atomic.LoadUint64(&x) == 3 {
			return errors.E("adder", errors.K.Invalid, "max", 3)
		}
		atomic.AddUint64(&x, 1)
		return nil
	}
	var err error
	wg := newWorkGroup("adder", 2, 1, true)
	i := 0
	for i = 0; i < 10; i++ {
		err = wg.Add(adder)
		time.Sleep(time.Millisecond * 10)
		if err != nil {
			break
		}
	}
	require.Error(t, err)
	err = wg.CloseWait()
	require.Error(t, err)
	require.Equal(t, uint64(3), x)
	require.Equal(t, uint64(4), exec)

	x = 0
	exec = 0
	wg = newWorkGroup("adder", 2, 1, true)
	for i = 0; i < 10; i++ {
		err = wg.Add(adder)
		time.Sleep(time.Millisecond * 2) // slow down a bit to make sure we see an error
		if err != nil {
			break
		}
	}
	require.Error(t, err)
	err = wg.CloseWait()
	require.Error(t, err)
	require.Equal(t, uint64(3), x)
	require.True(t, exec < uint64(10))
}
