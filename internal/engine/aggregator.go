package engine

import (
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Aggregator is the SHARED MUTABLE STATE of a run: one instance, written to by
// every worker goroutine concurrently, read once at the end by Summary(). It is
// the "aggregates metrics safely into a thread-safe map" requirement.
//
// It deliberately demonstrates BOTH of Go's low-level concurrency primitives so
// you can compare them:
//
//   - sync/atomic counters (total/success/failed): lock-free, ideal for simple
//     integer increments hammered by many goroutines. Like Interlocked.Increment
//     in .NET or AtomicLong in Java.
//
//   - a sync.Mutex guarding the map + slice: needed because map writes and slice
//     appends are NOT atomic and are not safe under concurrent access (a plain
//     `map[k]++` from two goroutines is a data race that `go test -race` will
//     flag, and the runtime may even panic with "concurrent map writes").
//
// Idiomatic alternative (see README): instead of locking shared state, have
// workers send Results down a channel to ONE aggregator goroutine that owns the
// map exclusively — "don't communicate by sharing memory; share memory by
// communicating." We use the lock here precisely because you asked to see a
// thread-safe map.
type Aggregator struct {
	// Lock-free counters.
	total   atomic.Int64
	success atomic.Int64
	failed  atomic.Int64

	// Everything below is protected by mu.
	mu         sync.Mutex
	statusHist map[int]int     // HTTP status code -> count
	errHist    map[string]int  // coarse error label -> count
	latencies  []time.Duration // every successful attempt's latency, for percentiles
}

// newAggregator returns the one Aggregator a Run shares with all its workers.
// It returns a pointer so every worker records into the same instance (and
// because the mutex inside must never be copied).
//
// The maps must be created here with make(). A map's zero value is nil, and
// writing to a nil map is a runtime error: Go calls it a panic, the equivalent
// of an unhandled exception, with the message "assignment to entry in nil
// map". Reading a nil map is fine and returns the zero value. The latencies
// slice is left at its zero value on purpose: a nil slice is safe to append to,
// because append allocates the backing array on first use.
func newAggregator() *Aggregator {
	return &Aggregator{
		statusHist: make(map[int]int),
		errHist:    make(map[string]int),
	}
}

// Record folds one Result into the running totals. It is safe to call from many
// goroutines simultaneously.
func (a *Aggregator) Record(r Result) {
	// Counters first — no lock required. `total++` would be three steps (read,
	// add, write) and two goroutines can interleave them, losing an update.
	// Add(1) is atomic: the CPU has instructions that do read-modify-write on a
	// single machine word in one indivisible step, so nobody can get in between.
	// That's also why atomics exist only for word-sized things: ints, pointers,
	// a flag. No waiting, no lock — just a different instruction.
	a.total.Add(1)
	if r.OK() {
		a.success.Add(1)
	} else {
		a.failed.Add(1)
	}

	// Map/slice mutations must hold the lock. A map write is far more than one
	// word: hash the key, find the bucket, maybe grow and rehash the table. No
	// CPU instruction covers that, so there is no atomic map update. The mutex
	// solves it differently: it doesn't make the steps indivisible, it makes
	// everyone else wait until we're done. Slower than an atomic, but it works
	// for any size of critical section.
	//
	// defer Unlock guarantees release even if something below were to panic —
	// the analog of try/finally.
	a.mu.Lock()
	defer a.mu.Unlock()

	if r.Err != nil {
		a.errHist[classifyErr(r.Err)]++
		return
	}
	a.statusHist[r.StatusCode]++
	a.latencies = append(a.latencies, r.Latency)
}

// Summary is the read-side snapshot computed once after all workers have
// finished. Because the workers are done by the time Summary() is called, the
// percentile math below does not need the lock — but we take it anyway to stay
// correct if the contract ever changes.
type Summary struct {
	Total      int64
	Success    int64
	Failed     int64
	StatusHist map[int]int
	ErrHist    map[string]int
	Min        time.Duration
	Max        time.Duration
	Mean       time.Duration
	P50        time.Duration
	P95        time.Duration
	P99        time.Duration
}

// Summary computes the final aggregated metrics, including latency percentiles.
func (a *Aggregator) Summary() Summary {
	a.mu.Lock()
	defer a.mu.Unlock()

	s := Summary{
		Total:      a.total.Load(),
		Success:    a.success.Load(),
		Failed:     a.failed.Load(),
		StatusHist: cloneIntMap(a.statusHist),
		ErrHist:    cloneStrMap(a.errHist),
	}

	if len(a.latencies) == 0 {
		return s
	}

	// Sort a copy so percentile lookups are simple index math. Sorting in place
	// would be fine here too since we're done, but copying keeps Summary pure.
	sorted := make([]time.Duration, len(a.latencies))
	copy(sorted, a.latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}

	s.Min = sorted[0]
	s.Max = sorted[len(sorted)-1]
	s.Mean = sum / time.Duration(len(sorted))
	s.P50 = percentile(sorted, 0.50)
	s.P95 = percentile(sorted, 0.95)
	s.P99 = percentile(sorted, 0.99)
	return s
}

// percentile returns the nearest-rank percentile from a pre-sorted slice.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p * float64(len(sorted)))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func classifyErr(err error) string {
	// Kept intentionally coarse; a real system would inspect with errors.As for
	// *net.OpError, context.DeadlineExceeded, etc. We do a bit of that in the CLI.
	msg := err.Error()
	switch {
	case strings.Contains(msg, "context deadline exceeded"), strings.Contains(msg, "Client.Timeout"):
		return "timeout"
	case strings.Contains(msg, "connection refused"):
		return "connection_refused"
	case strings.Contains(msg, "no such host"):
		return "dns_failure"
	default:
		return "other"
	}
}

func cloneIntMap(m map[int]int) map[int]int {
	out := make(map[int]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneStrMap(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
