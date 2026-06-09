// Package engine is the distributed health-check core: it streams URLs from a
// source, fans them out across a pool of worker goroutines over a channel,
// performs HTTP checks under strict per-request timeouts, and aggregates the
// results into a thread-safe store.
//
// This is the heart of the "learn Go concurrency" project. The mental-model
// translation from C#/Java:
//
//	Go                         C# / Java
//	------------------------   ------------------------------------------
//	goroutine                  Task / Thread (but far cheaper; millions OK)
//	channel                    BlockingCollection<T> / ArrayBlockingQueue
//	context.Context            CancellationToken (+ deadline)
//	ctx, cancel := WithTimeout new CancellationTokenSource(timeout)
//	select { case <-ch ... }   await Task.WhenAny / select on multiple sources
//	errgroup.Group             Task.WhenAll with first-error cancellation
//	sync.WaitGroup             CountdownEvent
//	go f()                     Task.Run(f) — but no Task handle; coordinate
//	                           via channels/WaitGroup instead of awaiting
//
// Topology (a classic fan-out / fan-in, minus the fan-in channel because the
// aggregator is itself concurrency-safe):
//
//	dispatcher ──jobs chan──▶ [worker 1..N] ──Record()──▶ Aggregator
package engine

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hemantksingh/go-starter/internal/config"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Engine is reusable and safe for concurrent Run calls (the *http.Client it
// holds is itself concurrency-safe and pools connections). Construct it once
// and inject it where needed — that's the manual-DI pattern.
type Engine struct {
	client  *http.Client
	workers int
	timeout time.Duration
	buffer  int
	log     *zap.Logger
}

// New builds an Engine from config. Note we do NOT set http.Client.Timeout;
// instead each request gets its own context deadline (see check), which lets a
// caller-supplied parent context cancel the whole run cooperatively.
func New(cfg config.EngineConfig, log *zap.Logger) *Engine {
	transport := &http.Transport{
		// Allow the pool to actually run in parallel: default
		// MaxIdleConnsPerHost is 2, which would serialize same-host checks.
		MaxIdleConns:        cfg.Workers * 2,
		MaxIdleConnsPerHost: cfg.Workers,
		IdleConnTimeout:     30 * time.Second,
	}
	return &Engine{
		client:  &http.Client{Transport: transport},
		workers: cfg.Workers,
		timeout: cfg.Timeout,
		buffer:  cfg.BufferSize,
		log:     log,
	}
}

// Run executes the full pipeline: it reads newline-delimited URLs from src,
// distributes them to the worker pool, and returns the aggregated Summary once
// every URL has been processed (or the context is cancelled).
//
// src is streamed line-by-line — we never hold all 10,000 URLs in memory at
// once, which is the point of using a channel as the hand-off buffer.
func (e *Engine) Run(ctx context.Context, src io.Reader) (Summary, error) {
	agg := newAggregator()
	jobs := make(chan string, e.buffer)

	// errgroup ties the goroutines together: the first non-nil error from any
	// of them cancels the shared ctx and Wait() returns that error. All the
	// goroutines below observe cancellation via this derived ctx.
	g, ctx := errgroup.WithContext(ctx)

	// --- Producer: one dispatcher goroutine -------------------------------
	g.Go(func() error {
		// Closing jobs is how we signal "no more work" to the workers ranging
		// over it. defer guarantees it happens even on a read error.
		defer close(jobs)
		return dispatch(ctx, src, jobs)
	})

	// --- Consumers: the worker pool ---------------------------------------
	for i := 0; i < e.workers; i++ {
		g.Go(func() error {
			return e.worker(ctx, jobs, agg)
		})
	}

	if err := g.Wait(); err != nil {
		return Summary{}, err
	}
	return agg.Summary(), nil
}

// dispatch streams URLs from src onto the jobs channel. The select makes the
// send cancellable: if ctx is cancelled while the channel is full and no worker
// is reading, we return promptly instead of blocking forever.
func dispatch(ctx context.Context, src io.Reader, jobs chan<- string) error {
	scanner := bufio.NewScanner(src)
	// URLs can be long; raise the line cap above the 64KB default just in case.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // skip blanks and comments
		}
		select {
		case jobs <- line:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return scanner.Err()
}

// worker pulls URLs off the jobs channel until it is closed and drained, or
// until the context is cancelled. Ranging over a channel is the idiomatic loop;
// we use select instead so cancellation is honored even mid-drain.
func (e *Engine) worker(ctx context.Context, jobs <-chan string, agg *Aggregator) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case url, ok := <-jobs:
			if !ok {
				return nil // channel closed and drained: clean exit
			}
			agg.Record(e.check(ctx, url))
		}
	}
}

// check performs a single GET under a strict per-request deadline derived from
// the parent ctx. This is the direct analog of:
//
//	using var cts = CancellationTokenSource.CreateLinkedTokenSource(parent);
//	cts.CancelAfter(timeout);
//	await httpClient.GetAsync(url, cts.Token);
//
// If the parent ctx is cancelled, reqCtx is too — so an in-flight request is
// aborted, not leaked.
func (e *Engine) check(ctx context.Context, url string) Result {
	start := time.Now()

	reqCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel() // always release the timer; failing to do so leaks resources

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return Result{URL: url, Err: err, Latency: time.Since(start)}
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return Result{URL: url, Err: err, Latency: time.Since(start)}
	}
	defer resp.Body.Close()

	// Drain the body so the TCP connection can be returned to the pool and
	// reused (keep-alive). Skipping this is a classic Go performance bug.
	_, _ = io.Copy(io.Discard, resp.Body)

	return Result{URL: url, StatusCode: resp.StatusCode, Latency: time.Since(start)}
}
