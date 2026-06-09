// learn/06 — worker pool: a FIXED number of workers sharing one job queue.
//
// Run it:   go run ./learn/06-worker-pool
//
// RECAP: the channels lesson spawned one goroutine PER item. Fine for 5; reckless for 10,000
// (10k simultaneous network connections). We want bounded concurrency.
//
// THE ONE IDEA: start a fixed pool of N workers, all receiving from the SAME
// jobs channel. Go hands each queued job to whichever worker is free — that's
// automatic load balancing, no dispatcher logic needed.
//
// In C# this is Parallel.ForEach with MaxDegreeOfParallelism = N, or a fixed
// set of consumers draining one BlockingCollection<T>.
//
// The shape — "fan-out" (one queue, many workers) then "fan-in" (many workers,
// one results channel) — is exactly what the real engine does.
package main

import (
	"fmt"
	"sync"
	"time"
)

const (
	numWorkers = 3
	numJobs    = 9
)

func main() {
	jobs := make(chan int)    // work flows in here
	results := make(chan int) // answers flow back out here

	// --- Fan-out: start N workers. They block on <-jobs until work arrives. ---
	var wg sync.WaitGroup
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go worker(w, jobs, results, &wg)
	}

	// --- Feed the queue, then close it to say "no more jobs coming." ---
	// This runs in its own goroutine so main can start receiving results
	// immediately (otherwise sending would block with no one reading).
	go func() {
		for j := 1; j <= numJobs; j++ {
			jobs <- j
		}
		close(jobs) // tells every worker's `range jobs` loop to end
	}()

	// --- Close results once all workers have exited (same idiom as the channels lesson). ---
	go func() {
		wg.Wait()
		close(results)
	}()

	// --- Fan-in: collect every result until results is closed. ---
	sum := 0
	for r := range results {
		sum += r
	}
	fmt.Printf("main: %d jobs handled by %d workers, sum of squares = %d\n", numJobs, numWorkers, sum)
}

// worker pulls jobs until the channel is closed and drained, then returns.
// `range jobs` is the idiomatic consumer loop: it yields each value and exits
// automatically when the channel is closed.
func worker(id int, jobs <-chan int, results chan<- int, wg *sync.WaitGroup) {
	defer wg.Done()
	for j := range jobs {
		time.Sleep(50 * time.Millisecond) // pretend the work takes a moment
		fmt.Printf("  worker %d handled job %d\n", id, j)
		results <- j * j
	}
}
