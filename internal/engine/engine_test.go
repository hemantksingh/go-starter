package engine

// Behavioural tests through the public entry point, Engine.Run, against an
// in-process httptest server. httptest is the Go analog of TestServer /
// WebApplicationFactory: a real HTTP server on a random loopback port, no
// mocks, torn down with Close. Keeping the server in-process means this
// package stays self-contained (it does not depend on cmd/testserver).

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hemantksingh/go-starter/internal/config"
	"go.uber.org/zap"
)

func newTestEngine(workers int, timeout time.Duration) *Engine {
	return New(config.EngineConfig{
		Workers:    workers,
		Timeout:    timeout,
		BufferSize: workers,
	}, zap.NewNop())
}

// statusServer answers every request with the status code named by the path,
// e.g. GET /503 -> 503. Unknown paths get 200.
func statusServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil {
			code = http.StatusOK
		}
		w.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_HappyPath(t *testing.T) {
	t.Parallel()
	srv := statusServer(t)

	var b strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, "%s/200\n", srv.URL)
	}
	for i := 0; i < 5; i++ {
		fmt.Fprintf(&b, "%s/500\n", srv.URL)
	}
	for i := 0; i < 3; i++ {
		fmt.Fprintf(&b, "%s/404\n", srv.URL)
	}

	s, err := newTestEngine(8, time.Second).Run(t.Context(), strings.NewReader(b.String()))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if s.Total != 28 || s.Success != 20 || s.Failed != 8 {
		t.Errorf("Total/Success/Failed = %d/%d/%d, want 28/20/8", s.Total, s.Success, s.Failed)
	}
	if s.StatusHist[200] != 20 || s.StatusHist[500] != 5 || s.StatusHist[404] != 3 {
		t.Errorf("StatusHist = %v, want 200:20 500:5 404:3", s.StatusHist)
	}
	if len(s.ErrHist) != 0 {
		t.Errorf("ErrHist = %v, want empty", s.ErrHist)
	}
	if s.Min <= 0 || s.Max < s.Min || s.P50 < s.Min || s.P99 > s.Max {
		t.Errorf("latency stats out of order: min=%v p50=%v p99=%v max=%v", s.Min, s.P50, s.P99, s.Max)
	}
}

func TestDispatch_SkipsBlanksAndComments(t *testing.T) {
	t.Parallel()
	src := strings.NewReader("\n# comment\nhttp://a\n   \n  http://b  \n#http://c\nhttp://d")

	jobs := make(chan string)
	errc := make(chan error, 1)
	go func() {
		defer close(jobs)
		errc <- dispatch(t.Context(), src, jobs)
	}()

	var got []string
	for u := range jobs {
		got = append(got, u)
	}
	if err := <-errc; err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	want := []string{"http://a", "http://b", "http://d"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("dispatched %v, want %v", got, want)
	}
}

// TestRun_PerRequestTimeout: individual checks exceed their deadline, are
// recorded as timeouts, and the run as a whole still SUCCEEDS. This is the
// per-request context, not a run-level failure.
func TestRun_PerRequestTimeout(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(2 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done(): // client gave up: stop so srv.Close() isn't held up
		}
	}))
	t.Cleanup(srv.Close)

	src := strings.Repeat(srv.URL+"\n", 4)
	s, err := newTestEngine(4, 50*time.Millisecond).Run(t.Context(), strings.NewReader(src))
	if err != nil {
		t.Fatalf("Run should succeed despite per-request timeouts, got: %v", err)
	}
	if s.Total != 4 || s.Failed != 4 || s.Success != 0 {
		t.Errorf("Total/Success/Failed = %d/%d/%d, want 4/0/4", s.Total, s.Success, s.Failed)
	}
	if got := s.ErrHist["timeout"]; got != 4 {
		t.Errorf("ErrHist[timeout] = %d (%v), want 4", got, s.ErrHist)
	}
}

// TestRun_ParentCancel: cancelling the caller's context aborts in-flight
// requests and makes Run return promptly with context.Canceled. The outer
// deadline distinguishes "honoured the cancel" from "hung until a timeout".
func TestRun_ParentCancel(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // hold every request open until the client cancels
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ctx, cancelRun := context.WithCancel(ctx)
	time.AfterFunc(100*time.Millisecond, cancelRun)

	src := strings.Repeat(srv.URL+"\n", 50)
	start := time.Now()
	_, err := newTestEngine(4, time.Minute).Run(ctx, strings.NewReader(src))
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("Run took %v after cancel; should return promptly", elapsed)
	}
}

func TestRun_ConnectionRefused(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing is listening on that port any more

	s, err := newTestEngine(2, time.Second).Run(t.Context(), strings.NewReader(url+"\n"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if s.Total != 1 || s.Failed != 1 {
		t.Errorf("Total/Failed = %d/%d, want 1/1", s.Total, s.Failed)
	}
	if got := s.ErrHist["connection_refused"]; got != 1 {
		t.Errorf("ErrHist[connection_refused] = %d (%v), want 1", got, s.ErrHist)
	}
}
