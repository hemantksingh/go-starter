package engine

// These tests live in `package engine` (not `engine_test`), so they can reach
// unexported names like newAggregator and classifyErr. That's the Go norm for
// unit tests of a package's internals; black-box tests use the `_test` suffix.
//
// No assertion library: the convention is `if got != want { t.Errorf(...) }`.
// Table-driven tests with t.Run subtests are the Go form of xUnit's
// [Theory]/[InlineData].

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestResult_OK(t *testing.T) {
	tests := []struct {
		name string
		r    Result
		want bool
	}{
		{"200 is healthy", Result{StatusCode: 200}, true},
		{"399 is healthy (boundary)", Result{StatusCode: 399}, true},
		{"400 is not (boundary)", Result{StatusCode: 400}, false},
		{"199 is not", Result{StatusCode: 199}, false},
		{"500 is not", Result{StatusCode: 500}, false},
		{"error wins over a good status", Result{StatusCode: 200, Err: errors.New("x")}, false},
		{"zero value is not", Result{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.OK(); got != tt.want {
				t.Errorf("OK() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAggregator_ConcurrentRecord is the test the race detector exists for.
// Many goroutines hammer Record; `go test -race` proves the atomics and the
// mutex are placed correctly, and the final counts prove no update was lost.
func TestAggregator_ConcurrentRecord(t *testing.T) {
	const (
		goroutines = 50
		perG       = 200
	)
	agg := newAggregator()

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				switch i % 4 {
				case 0, 1:
					agg.Record(Result{StatusCode: 200, Latency: time.Millisecond})
				case 2:
					agg.Record(Result{StatusCode: 500, Latency: time.Millisecond})
				case 3:
					agg.Record(Result{Err: errors.New("context deadline exceeded")})
				}
			}
		}()
	}
	wg.Wait()

	s := agg.Summary()
	total := int64(goroutines * perG)
	if s.Total != total {
		t.Errorf("Total = %d, want %d", s.Total, total)
	}
	if want := total / 2; s.Success != want {
		t.Errorf("Success = %d, want %d", s.Success, want)
	}
	if want := total / 2; s.Failed != want {
		t.Errorf("Failed = %d, want %d", s.Failed, want)
	}
	if got, want := s.StatusHist[200], int(total/2); got != want {
		t.Errorf("StatusHist[200] = %d, want %d", got, want)
	}
	if got, want := s.StatusHist[500], int(total/4); got != want {
		t.Errorf("StatusHist[500] = %d, want %d", got, want)
	}
	if got, want := s.ErrHist["timeout"], int(total/4); got != want {
		t.Errorf("ErrHist[timeout] = %d, want %d", got, want)
	}
}

func TestAggregator_SummaryPercentiles(t *testing.T) {
	agg := newAggregator()
	// Ten latencies, 10ms..100ms, recorded out of order to prove Summary sorts.
	for _, ms := range []int{70, 10, 100, 40, 90, 20, 60, 30, 80, 50} {
		agg.Record(Result{StatusCode: 200, Latency: time.Duration(ms) * time.Millisecond})
	}
	s := agg.Summary()

	// percentile() is index math: sorted[int(p*len)], not textbook nearest-rank.
	// With len=10: P50 -> sorted[5] = 60ms, P95 -> sorted[9] = 100ms, P99 -> sorted[9].
	want := map[string]time.Duration{
		"Min":  10 * time.Millisecond,
		"Max":  100 * time.Millisecond,
		"Mean": 55 * time.Millisecond,
		"P50":  60 * time.Millisecond,
		"P95":  100 * time.Millisecond,
		"P99":  100 * time.Millisecond,
	}
	got := map[string]time.Duration{
		"Min": s.Min, "Max": s.Max, "Mean": s.Mean,
		"P50": s.P50, "P95": s.P95, "P99": s.P99,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %v, want %v", k, got[k], w)
		}
	}
}

func TestAggregator_SummaryEmpty(t *testing.T) {
	s := newAggregator().Summary()
	if s.Total != 0 || s.Min != 0 || s.P99 != 0 {
		t.Errorf("empty Summary = %+v, want all zero", s)
	}
	// Maps must be non-nil so callers can index them without a nil check.
	if s.StatusHist == nil || s.ErrHist == nil {
		t.Error("empty Summary has nil histogram maps")
	}
}

func TestAggregator_SummaryIsSnapshot(t *testing.T) {
	agg := newAggregator()
	agg.Record(Result{StatusCode: 200})
	s := agg.Summary()
	s.StatusHist[200] = 99 // mutate the snapshot...
	if got := agg.Summary().StatusHist[200]; got != 1 {
		t.Errorf("mutating Summary leaked into Aggregator: got %d, want 1", got)
	}
}

func TestClassifyErr(t *testing.T) {
	tests := []struct {
		name string
		err  string
		want string
	}{
		{"context deadline", "Get \"http://x\": context deadline exceeded", "timeout"},
		{"client timeout", "Client.Timeout exceeded while awaiting headers", "timeout"},
		{"connection refused", "dial tcp 127.0.0.1:1: connect: connection refused", "connection_refused"},
		{"no such host", "dial tcp: lookup nope.invalid: no such host", "dns_failure"},
		{"unrecognised", "something else entirely", "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyErr(errors.New(tt.err)); got != tt.want {
				t.Errorf("classifyErr(%q) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}
