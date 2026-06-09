// learn/04 — goroutines: running things concurrently.
//
// Run it:   go run ./learn/04-goroutines
//
// THE ONE IDEA: put the word `go` in front of a function call and it runs
// concurrently — in a "goroutine" — instead of blocking. A goroutine is like a
// Task in C# (or a Thread), but so cheap you can start thousands without thinking.
//
// THE ONE GOTCHA: `go f()` returns immediately. main() does NOT wait for it.
// If main() returns, the whole program exits and your goroutines are killed
// mid-flight. So we need a way to wait — that's what sync.WaitGroup is for
// (think CountdownEvent / a join). We'll use channels for that job later; for
// now, WaitGroup is the simplest "wait for N things to finish."
package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	// A WaitGroup is a counter. Add(n) before starting work, Done() when each
	// piece finishes, Wait() blocks until the counter hits zero.
	var wg sync.WaitGroup

	fmt.Println("main: starting 5 workers")

	for i := 1; i <= 5; i++ {
		wg.Add(1) // "I'm about to start one more thing"

		// `go func(){...}()` launches an anonymous function as a goroutine.
		// We pass `i` in as an argument (id) on purpose — capturing the loop
		// variable directly is a classic bug source in older Go.
		go func(id int) {
			defer wg.Done() // "this thing is finished" — runs when the func returns

			// Pretend to do work. time.Sleep just parks THIS goroutine; the
			// others keep running. That's the concurrency.
			time.Sleep(time.Duration(id) * 100 * time.Millisecond)
			fmt.Printf("  worker %d done\n", id)
		}(i)
	}

	fmt.Println("main: all workers launched, now waiting...")
	wg.Wait() // block here until all 5 have called Done()
	fmt.Println("main: everyone finished, exiting")
}
