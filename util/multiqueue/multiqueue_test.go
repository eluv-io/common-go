package multiqueue_test

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/multiqueue"
)

func TestEmpty(t *testing.T) {
	mq := multiqueue.New()
	addPushPopRemove(t, mq)
}

func TestBasic(t *testing.T) {
	inputs := make([]multiqueue.Input, 10)

	mq := multiqueue.New()
	wg := &sync.WaitGroup{}
	for i := 0; i < len(inputs); i++ {
		inputs[i] = mq.NewInput(10)
		wg.Add(1)
		go func(num int, in multiqueue.Input) {
			for j := 0; j < 10; j++ {
				in.Push(&msg{num, j})
			}
			wg.Done()
		}(i, inputs[i])
	}

	wg.Wait()

	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			res, closed := mq.Pop()
			require.False(t, closed)
			msg := res.(*msg)
			require.Equal(t, j, msg.cat, msg)
			require.Equal(t, i, msg.seq, msg)
		}
	}

	for _, i := range rand.Perm(10) {
		singlePush(t, i, mq, inputs[i])
	}

	for _, input := range inputs {
		input.Close()
	}

	addPushPopRemove(t, mq)
}

func TestAddRemove(t *testing.T) {
	mq := multiqueue.New()

	res := make(chan interface{}, 20)
	started := make(chan interface{})
	done := make(chan interface{})

	go func() {
		close(started)
		var v interface{}
		closed := false
		for !closed {
			v, closed = mq.Pop()
			if !closed {
				res <- v
			}
		}
		close(done)
	}()

	<-started
	require.Equal(t, 0, len(res))

	time.Sleep(10 * time.Millisecond)

	in1 := mq.NewInput(1)
	in1.Push(msg{1, 1})
	require.Equal(t, msg{1, 1}, <-res)

	time.Sleep(10 * time.Millisecond)

	in2 := mq.NewInput(1)
	in2.Push(msg{2, 1})
	require.Equal(t, msg{2, 1}, <-res)

	in1.Close()

	time.Sleep(10 * time.Millisecond)

	in2.Push(msg{2, 2})
	require.Equal(t, msg{2, 2}, <-res)

	in2.Close()

	time.Sleep(10 * time.Millisecond)

	err := mq.Close()
	require.NoError(t, err)

	<-done
}

const (
	numClients = 50
	numMsgs    = 1
)

// BenchamrkConcurrent creates and runs X test "clients" concurrently, each of which
// executes the following:
//   - add a new input
//   - submit a number of messages
//   - close the input
//
// The test verifies that for each test routine all messages are popped from the
// MultiQueue in order and that all inputs are properly removed.
func BenchmarkConcurrent(b *testing.B) {
	type client struct {
		id              int
		in              multiqueue.Input
		nextExpectedNum int // last msg number popped from queue
	}

	b.StopTimer()

	mq := multiqueue.New()
	clients := make([]*client, numClients)

	wg := &sync.WaitGroup{}
	run := func(c *client) {
		for i := 0; i < numMsgs; i++ {
			c.in.Push(&msg{c.id, i})
		}
		c.in.Close()
		wg.Done()
	}

	b.StartTimer()
	for bn := 0; bn < b.N; bn++ {
		for i := 0; i < numClients; i++ {
			clients[i] = &client{
				id: i,
				in: mq.NewInput(5 + rand.Intn(16)), // 5-20 queue cap
			}
		}

		for _, client := range clients {
			wg.Add(1)
			go run(client)
		}

		for i := 0; i < numClients*numMsgs; i++ {
			m, closed := mq.Pop()
			require.False(b, closed)
			c := clients[m.(*msg).cat]
			require.Equal(b, c.nextExpectedNum, m.(*msg).seq)
			c.nextExpectedNum++
		}

		for _, client := range clients {
			require.Equal(b, numMsgs, client.nextExpectedNum)
		}

		wg.Wait()
	}

	b.StopTimer()
	b.SetBytes(numClients * numMsgs)

	// input queue removal in MultiQueue is asynchronous, so we can't test it
	// here. Instead, run another add/push/pop/remove cycle, and the verify that
	// we have at most 1 queue left (from the add/push/pop/remove...)
	addPushPopRemove(b, mq)
	require.LessOrEqual(b, mq.InputCount(), 1)
}

// BenchmarkSimpleChannel is like BenchmarkConcurrent, but using a simple
// channel instead of a MultiQueue.
func BenchmarkSimpleChannel(b *testing.B) {
	type client struct {
		id              int
		nextExpectedNum int // last msg number popped from queue
	}

	b.StopTimer()

	channel := make(chan *msg, 12*numClients)
	clients := make([]*client, numClients)

	wg := &sync.WaitGroup{}
	run := func(c *client) {
		for i := 0; i < numMsgs; i++ {
			channel <- &msg{c.id, i}
		}
		wg.Done()
	}

	b.StartTimer()
	for bn := 0; bn < b.N; bn++ {
		for i := 0; i < numClients; i++ {
			clients[i] = &client{id: i}
		}

		for _, client := range clients {
			wg.Add(1)
			go run(client)
		}

		for i := 0; i < numClients*numMsgs; i++ {
			m := <-channel
			c := clients[m.cat]
			require.Equal(b, c.nextExpectedNum, m.seq)
			c.nextExpectedNum++
		}

		for _, client := range clients {
			require.Equal(b, numMsgs, client.nextExpectedNum)
		}

		b.SetBytes(numClients * numMsgs)

		wg.Wait()
	}

	b.StopTimer()
	b.SetBytes(numClients * numMsgs)

	require.Equal(b, 0, len(channel))
}

// addPushPopRemove adds a new input, pushes a single message, pops it and closes
// the input.
func addPushPopRemove(t require.TestingT, mq multiqueue.MultiQueue) {
	res := make(chan interface{}, 1)
	started := make(chan interface{})
	done := make(chan interface{})

	go func() {
		close(started)
		v, _ := mq.Pop()
		res <- v
		close(done)
	}()

	<-started
	require.Equal(t, 0, len(res))

	time.Sleep(10 * time.Millisecond)

	m := &msg{1, 1}
	mq.NewInput(1).Push(m)

	<-done
	require.Equal(t, m, <-res)
}

// singlePush pushes a single value to the given input and expects it to be
// popped from the multi-queue.
func singlePush(t *testing.T, idx int, mq multiqueue.MultiQueue, input multiqueue.Input) {

	c := make(chan interface{}, 1)

	go func() {
		res, closed := mq.Pop()
		require.False(t, closed)
		c <- res
	}()

	require.Equal(t, 0, len(c))
	m := &msg{rand.Int(), rand.Int()}
	input.Push(m)

	res := (<-c).(*msg)
	require.Equal(t, m, res)

	close(c)
}

type msg struct {
	cat int // category, e.g. client ID
	seq int // sequence number
}

func (m *msg) String() string {
	return fmt.Sprintf("%d-%d", m.cat, m.seq)
}
