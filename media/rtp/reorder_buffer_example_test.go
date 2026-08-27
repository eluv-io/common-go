package rtp_test

import (
	"fmt"
	"time"

	"github.com/eluv-io/common-go/media/rtp"
)

// ExampleReorderBuffer demonstrates typical usage: packets arriving out of order are held and released once the
// gap in front of them fills in, and a packet that never arrives is declared lost once its wait budget elapses.
//
// It also demonstrates the calling convention for Expire: the buffer has no timer of its own, so the caller re-arms
// a single timer to Deadline() after every Push (nothing to arm when ok is false) and calls Expire only when that
// timer fires. There is no fixed poll period and no need to call Expire on every packet - the deadline the buffer
// reports is always the correct one for whatever gap is currently open. This example has no real timer to fire, so
// it stands in for "the timer fired" by jumping the clock straight to the reported Deadline().
func ExampleReorderBuffer() {
	// maxWindow=4, maxWait=20ms, maxJump=0 (use the package default).
	b := rtp.NewReorderBuffer[string](4, 20*time.Millisecond, 0)

	now := time.Now()
	var deadline time.Time
	push := func(seq uint16, payload string) {
		released := b.Push(now, seq, payload, nil)
		// Re-arm (or disarm) the timer after every Push, exactly as a real caller would.
		var ok bool
		deadline, ok = b.Deadline()
		fmt.Printf("push %d/%s: released %v, armed %v\n", seq, payload, released, ok)
	}

	push(1, "a") // arrives on time: released immediately, nothing held: disarmed
	push(3, "c") // arrives ahead of the still-missing 2: held, first item since empty: armed
	push(4, "d") // also ahead: held, but window wasn't empty: deadline unchanged, still armed
	push(2, "b") // fills the gap: releases b, then cascades c and d behind it; window empty again: disarmed
	push(6, "f") // held: waiting on 5, first item since empty: armed

	// Packet 5 is lost for good. A real caller's timer fires at the deadline captured by the last push above;
	// simulate that by calling Expire at exactly that time instead of guessing an offset.
	released := b.Expire(deadline, nil)
	fmt.Printf("expire: released %v\n", released)

	stats := b.Stats()
	fmt.Println("reordered:", stats.Reordered, "lost:", stats.LostAfterTimeout)

	// Output:
	// push 1/a: released [a], armed false
	// push 3/c: released [], armed true
	// push 4/d: released [], armed true
	// push 2/b: released [b c d], armed false
	// push 6/f: released [], armed true
	// expire: released [f]
	// reordered: 3 lost: 1
}
