// learn/07 — context: timeouts and cancellation across goroutines.
//
// Run it:   go run ./learn/07-context
//
// RECAP: the worker-pool lesson's "work" was an uninterruptible time.Sleep. Real work (a slow
// HTTP call) must be cancellable — you can't let one stuck job hang forever.
//
// THE ONE IDEA: context.Context is a cancellation signal you pass down the call
// chain. It's C#'s CancellationToken; context.WithTimeout is a
// CancellationTokenSource with CancelAfter(...). When the deadline hits (or you
// call cancel()), ctx.Done() "fires" and every goroutine watching it bails out.
//
// THE PATTERN you'll see everywhere in Go:
//   select {
//   case <-workFinished:  // the happy path
//   case <-ctx.Done():    // cancelled or timed out -> stop now
//   }
//
// TWO RULES:
//   1. Pass ctx as the FIRST argument: func F(ctx context.Context, ...).
//   2. ALWAYS `defer cancel()` on a context you create — it releases the timer.
package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

func main() {
	// One deadline for the whole batch. CancellationTokenSource(500ms).
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Jobs of varying duration. Anything taking >= 500ms will be cut off.
	durations := map[int]time.Duration{
		1: 100 * time.Millisecond,
		2: 200 * time.Millisecond,
		3: 300 * time.Millisecond,
		4: 600 * time.Millisecond, // too slow -> cancelled
		5: 900 * time.Millisecond, // too slow -> cancelled
	}

	var wg sync.WaitGroup
	for id, dur := range durations {
		wg.Add(1)
		go func(id int, dur time.Duration) {
			defer wg.Done()
			start := time.Now()
			err := process(ctx, dur)
			elapsed := time.Since(start).Round(10 * time.Millisecond)

			switch {
			case err == nil:
				fmt.Printf("  job %d: done in %s\n", id, elapsed)
			case errors.Is(err, context.DeadlineExceeded):
				// errors.Is unwraps the chain to test identity — like checking
				// the exception type in a catch.
				fmt.Printf("  job %d: CANCELLED after %s (deadline hit)\n", id, elapsed)
			default:
				fmt.Printf("  job %d: failed: %v\n", id, err)
			}
		}(id, dur)
	}

	wg.Wait()
	fmt.Println("main: batch finished")
}

// process simulates cancellable work. The select races the work against the
// context: whichever channel fires first wins. If ctx is cancelled mid-flight,
// we return its error instead of waiting for the work to finish.
func process(ctx context.Context, work time.Duration) error {
	select {
	case <-time.After(work):
		return nil // finished before the deadline
	case <-ctx.Done():
		return ctx.Err() // context.DeadlineExceeded (or context.Canceled)
	}
}
