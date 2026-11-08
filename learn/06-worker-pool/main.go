// learn/06 — worker pool: a FIXED number of workers sharing one job queue.
//
// Run it:   go run ./learn/06-worker-pool
//
// THE PROBLEM: the channels lesson started one goroutine per item. With 5 items
// that's fine. Now imagine 10,000 URLs to check: `go check(url)` in a loop
// starts 10,000 goroutines at once, which means 10,000 simultaneous HTTP
// connections. Your machine runs out of sockets, the target gets hammered,
// and timeouts cascade. Starting the goroutines is cheap; what they DO isn't.
//
// What we want: all 10,000 get done, but only N are in flight at any moment.
// That's "bounded concurrency": a cap on how much runs at once.
//
// THE ONE IDEA: don't start a goroutine per job. Start N workers up front, and
// put the jobs on a channel they all read from. Each worker takes a job, does
// it, takes the next. The channel is the queue; Go's scheduler hands each job
// to whichever worker is free, so you get load balancing for free. At most N
// things run at a time, however many jobs there are.
//
// Note the direction flip from lesson 05. There, goroutines SENT results out.
// Here, jobs flow IN to a fixed set of goroutines. Both channels still appear:
// jobs in, results out — "fan-out" then "fan-in". The real engine is exactly
// this shape with N read from config.
//
// Also new: `jobs <-chan int` and `results chan<- int` in worker's signature
// are RECEIVE-ONLY and SEND-ONLY channel types. The compiler stops a worker
// from closing or sending on jobs — the arrow says which end it may touch.
//
// C# map: Parallel.ForEach with MaxDegreeOfParallelism = N, or N consumers
// draining one BlockingCollection<T>.
package main

import (
	"fmt"
	"sync"
	"time"
)

const (
	numWorkers = 3 // the cap: never more than 3 jobs in flight
	numJobs    = 9 // the work: more jobs than workers, so the queue matters
)

func main() {
	jobs := make(chan int)    // the queue: jobs flow IN to the workers
	results := make(chan int) // answers flow OUT, back to main

	// --- The cap: start exactly N workers, before any work exists. ---
	// Compare lesson 05, which started a goroutine per item. These three
	// are the only goroutines that will ever do work; they block on
	// `range jobs` until something is queued.
	var wg sync.WaitGroup
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go worker(w, jobs, results, &wg)
	}

	// --- The queue: push all 9 jobs, then close to say "no more coming." ---
	// Nothing here decides which worker gets which job. Whichever worker is
	// free takes the next one — the scheduler does the load balancing.
	// This runs in its own goroutine so main is free to receive results;
	// with an unbuffered channel, a send blocks until someone reads.
	go func() {
		for j := 1; j <= numJobs; j++ {
			jobs <- j
		}
		close(jobs) // every worker's `range jobs` loop ends once it's drained
	}()

	// --- Close results once all workers have exited (lesson 05's idiom). ---
	go func() {
		wg.Wait()
		close(results)
	}()

	// --- Fan-in: collect every answer until results is closed. ---
	// Watch the output: jobs complete in waves of 3, every 50ms. That's
	// the cap doing its job. 9 jobs, never more than 3 at a time.
	sum := 0
	for r := range results {
		sum += r
	}
	fmt.Printf("main: %d jobs handled by %d workers, sum of squares = %d\n", numJobs, numWorkers, sum)
}

// worker is the loop each of the N workers runs: take a job, do it, take the
// next. `range jobs` yields each value and exits when the channel is closed
// and drained, so the worker returns and wg.Done fires.
//
// The arrows in the signature are the compiler enforcing the direction:
// `<-chan int` means this function may only receive from jobs;
// `chan<- int` means it may only send to results. Try `close(jobs)` in
// here — it won't compile.
func worker(id int, jobs <-chan int, results chan<- int, wg *sync.WaitGroup) {
	defer wg.Done()
	for j := range jobs {
		time.Sleep(50 * time.Millisecond) // pretend the work takes a moment
		fmt.Printf("  worker %d handled job %d\n", id, j)
		results <- j * j
	}
}
