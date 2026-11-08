// learn/08 — shared state: combining results from many goroutines SAFELY.
//
// Run it:        go run ./learn/08-shared-state
// Run it right:  go run -race ./learn/08-shared-state   (proves no data race)
//
// RECAP: the channels/worker-pool lessons passed results back through a channel — the CSP way, no lock
// needed. Sometimes that's awkward: here, 100 workers each want to bump a shared
// counter and a shared histogram map. Funneling every increment through a
// channel is overkill. So we share the state directly — and protect it.
//
// THE PROBLEM: concurrent writes to the same memory are a DATA RACE. Two
// goroutines doing `hist[code]++` at once can lose updates, and Go's runtime may
// even panic with "concurrent map writes". `go run -race` detects this.
//
// TWO TOOLS (pick per field):
//
//	sync/atomic   lock-free integer ops. atomic.Int64.Add ≈ Interlocked.Increment.
//	sync.Mutex    a lock around a critical section. mu.Lock()/Unlock() ≈ lock(obj){}.
//	              (C#'s ConcurrentDictionary is roughly "a map with the lock built in".)
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Counter is a thread-safe aggregator — a miniature of internal/engine's Aggregator.
type Counter struct {
	total atomic.Int64 // hammered by every worker; lock-free is ideal here

	mu   sync.Mutex  // guards everything below
	hist map[int]int // status code -> count; map writes are NOT safe without this
}

func newCounter() *Counter {
	// A nil map panics on write, so initialize it here.
	return &Counter{hist: make(map[int]int)}
}

// Record is safe to call from any number of goroutines at once.
func (c *Counter) Record(code int) {
	c.total.Add(1) // no lock: atomic handles the race for a single integer

	c.mu.Lock()         // enter the critical section
	defer c.mu.Unlock() // defer == C#'s finally; always releases the lock
	c.hist[code]++
}

func main() {
	const (
		workers       = 100
		perWorker     = 100
		totalExpected = workers * perWorker // 10,000
	)

	c := newCounter()
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				c.Record(200 + i%3) // records codes 200, 201, 202
			}
		}()
	}
	wg.Wait()

	// If the locking is correct, total is exactly 10,000 and the histogram sums
	// to the same. A race would make these numbers come out wrong (or crash).
	sum := 0
	for _, n := range c.hist {
		sum += n
	}
	fmt.Printf("total (atomic) : %d  (expected %d)\n", c.total.Load(), totalExpected)
	fmt.Printf("histogram      : %v  (sums to %d)\n", c.hist, sum)
}
