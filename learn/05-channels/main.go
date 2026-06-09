// learn/05 — channels: getting RESULTS back from goroutines.
//
// Run it:   go run ./learn/05-channels
//
// RECAP (goroutines lesson): a WaitGroup tells you WHEN goroutines finish — but not WHAT
// they produced. The moment you want an answer back, WaitGroup isn't enough.
//
// THE ONE IDEA: a channel is a typed, thread-safe pipe between goroutines.
//   ch <- v      send v into the channel
//   v := <-ch    receive the next value out of it
// It's like a BlockingCollection<T> (with bounded capacity) — but it's also how
// goroutines synchronize. On an UNBUFFERED channel, a send blocks until some
// other goroutine is ready to receive (and vice-versa). That hand-off is the
// safe alternative to sharing a variable + a lock.
//
// TWO GOTCHAS:
//  1. `range ch` keeps receiving until the channel is CLOSED. If nobody closes
//     it, the range blocks forever (deadlock). So someone must close(ch).
//  2. Only the SENDER side should close — and only once. Closing signals "no
//     more values coming," it is not a "I'm done reading" message.
package main

import (
	"fmt"
	"sync"
)

func main() {
	// A channel carrying ints. `make` creates it (a nil channel blocks forever).
	results := make(chan int)

	var wg sync.WaitGroup
	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			// "Do work," then send the answer back through the pipe. This send
			// blocks until main's loop below is ready to receive it.
			results <- n * n
		}(i)
	}

	// Who closes the channel? Not main (it's busy receiving), and not any single
	// worker (there are five; closing twice panics). The idiom: one extra
	// goroutine waits for ALL workers via the WaitGroup, then closes once.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Receive until the channel is closed and drained. `range` over a channel is
	// the clean way to consume "an unknown number of values, then stop."
	sum := 0
	for r := range results {
		fmt.Printf("  received %d\n", r)
		sum += r
	}

	fmt.Printf("main: all results in, sum of squares = %d\n", sum)
}
