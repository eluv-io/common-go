package multiqueue

// AsChannel wraps the MultiQueue output as a receive channel by creating a go
// routine that reads from the MultiQueue and forwards all input to the channel.
// The channel is closed when the MultiQueue is closed.
func AsChannel(mq MultiQueue) <-chan interface{} {
	c := make(chan interface{})
	go func() {
		for {
			res, closed := mq.Pop()
			if closed {
				close(c)
				return
			}
			c <- res
		}
	}()
	return c
}
