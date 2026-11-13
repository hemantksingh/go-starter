// learn/09 — the engine: every concept wired into one real thing.
//
// Run it:        go run ./learn/09-the-engine
// Run it right:  go run -race ./learn/09-the-engine
//
// NO new concepts. This is lessons 04-08 assembled into a working health-check
// engine — the same shape as internal/engine/, just smaller and self-contained
// (it starts its own in-process target server, so no setup, no network).
//
//	lesson 04 goroutines + WaitGroup -> the worker pool's lifecycle
//	lesson 05 channels               -> the jobs queue
//	lesson 06 worker pool            -> fixed N workers draining that queue
//	lesson 07 context               -> per-request timeout in check()
//	lesson 08 shared state          -> the mutex+atomic aggregator
//
// Read this top-to-bottom, then open internal/engine/ — you'll recognize all of it.
package main

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	// A target server living inside this process. Its handler randomly returns
	// 500s and variable delays, so the metrics show healthy/unhealthy/timeouts.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rand.Intn(10) < 2 { // ~20% error
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		select {
		case <-time.After(time.Duration(rand.Intn(400)) * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done(): // client (us) cancelled -> stop work
		}
	}))
	defer srv.Close()

	// The "file of URLs" — here just a slice pointing at our test server.
	urls := make([]string, 300)
	for i := range urls {
		urls[i] = srv.URL
	}

	agg := run(context.Background(), urls, 30, 250*time.Millisecond)
	agg.print()
}

// run is the whole pipeline: dispatcher -> jobs channel -> worker pool -> aggregator.
func run(ctx context.Context, urls []string, workers int, timeout time.Duration) *agg {
	a := &agg{status: map[int]int{}}
	jobs := make(chan string, workers) // lesson 05 channel, buffered (see concurrency-model.md)
	client := &http.Client{}           // no Timeout field; we use context (lesson 07)

	// lesson 06: a FIXED pool of workers, all draining the same jobs channel.
	var wg sync.WaitGroup // lesson 04
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for url := range jobs { // ranges until jobs is closed
				a.record(check(ctx, client, url, timeout))
			}
		}()
	}

	// dispatcher: feed the queue, then close it so the workers' range loops end.
	for _, u := range urls {
		jobs <- u
	}
	close(jobs)

	wg.Wait() // block until every worker has drained and returned
	return a
}

// check does one GET under a per-request deadline (lesson 07).
func check(ctx context.Context, c *http.Client, url string, timeout time.Duration) result {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, _ := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	resp, err := c.Do(req)
	if err != nil {
		return result{err: err}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body) // drain for connection reuse
	return result{code: resp.StatusCode}
}

type result struct {
	code int
	err  error
}

// agg is the thread-safe sink (lesson 08): atomic counters + a mutex-guarded map.
type agg struct {
	total, ok atomic.Int64
	mu        sync.Mutex
	status    map[int]int
}

func (a *agg) record(r result) {
	a.total.Add(1)
	if r.err == nil && r.code < 400 {
		a.ok.Add(1)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.err == nil {
		a.status[r.code]++
	}
}

func (a *agg) print() {
	fmt.Printf("checked %d, healthy %d, unhealthy %d\n",
		a.total.Load(), a.ok.Load(), a.total.Load()-a.ok.Load())
	fmt.Printf("status codes: %v\n", a.status)
}
