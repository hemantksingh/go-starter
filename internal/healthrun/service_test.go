package healthrun_test

// External test package (healthrun_test) so it can import store/memory, which
// itself imports healthrun — an in-package test would be an import cycle.

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/hemantksingh/go-starter/internal/engine"
	"github.com/hemantksingh/go-starter/internal/healthrun"
	"github.com/hemantksingh/go-starter/internal/store/memory"
	"go.uber.org/zap"
)

// fakeRunner stands in for the engine. With block == nil it returns at once;
// otherwise it waits until the test closes block or the ctx is cancelled —
// which is how the tests make "in progress" deterministic.
type fakeRunner struct {
	block   chan struct{}
	err     error
	summary engine.Summary
}

func (f *fakeRunner) Run(ctx context.Context, _ io.Reader) (engine.Summary, error) {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return engine.Summary{}, ctx.Err()
		}
	}
	return f.summary, f.err
}

func newService(t *testing.T, r healthrun.Runner, maxURLs int) *healthrun.Service {
	t.Helper()
	svc := healthrun.NewService(memory.NewRunStore(), r, maxURLs, zap.NewNop())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = svc.Shutdown(ctx)
	})
	return svc
}

// waitStatus polls until the run reaches want or the deadline passes. Polling
// with a deadline beats time.Sleep: fast when it works, loud when it doesn't.
func waitStatus(t *testing.T, svc *healthrun.Service, tenant, id string, want healthrun.Status) healthrun.Run {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := svc.GetRun(context.Background(), tenant, id)
		if err != nil {
			t.Fatalf("GetRun: %v", err)
		}
		if run.Status == want {
			return run
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("run %s never reached %s", id, want)
	return healthrun.Run{}
}

func TestStartRun_Lifecycle(t *testing.T) {
	svc := newService(t, &fakeRunner{summary: engine.Summary{Total: 2, Success: 2}}, 100)

	run, err := svc.StartRun(context.Background(), "acme", 0, []string{"http://a", "http://b"})
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != healthrun.StatusQueued || run.URLCount != 2 || run.ID == "" {
		t.Errorf("initial run = %+v", run)
	}

	done := waitStatus(t, svc, "acme", run.ID, healthrun.StatusDone)
	if done.Summary == nil || done.Summary.Total != 2 {
		t.Errorf("Summary = %+v, want Total 2", done.Summary)
	}
	if done.StartedAt == nil || done.FinishedAt == nil {
		t.Error("timestamps not set")
	}
}

func TestStartRun_RunnerFailure(t *testing.T) {
	svc := newService(t, &fakeRunner{err: errors.New("boom")}, 100)
	run, _ := svc.StartRun(context.Background(), "acme", 0, []string{"http://a"})

	failed := waitStatus(t, svc, "acme", run.ID, healthrun.StatusFailed)
	if failed.Error != "boom" || failed.Summary != nil {
		t.Errorf("failed run = %+v", failed)
	}
}

func TestStartRun_Caps(t *testing.T) {
	svc := newService(t, &fakeRunner{}, 3)
	ctx := context.Background()
	four := []string{"a", "b", "c", "d"}

	if _, err := svc.StartRun(ctx, "acme", 0, nil); !errors.Is(err, healthrun.ErrNoURLs) {
		t.Errorf("empty: err = %v, want ErrNoURLs", err)
	}
	if _, err := svc.StartRun(ctx, "acme", 0, four); !errors.Is(err, healthrun.ErrTooManyURLs) {
		t.Errorf("over global cap: err = %v, want ErrTooManyURLs", err)
	}
	// A tenant override replaces the global cap in either direction.
	if _, err := svc.StartRun(ctx, "globex", 10, four); err != nil {
		t.Errorf("under tenant cap: err = %v, want nil", err)
	}
	if _, err := svc.StartRun(ctx, "initech", 1, four[:2]); !errors.Is(err, healthrun.ErrTooManyURLs) {
		t.Errorf("over tenant cap: err = %v, want ErrTooManyURLs", err)
	}
}

func TestStartRun_OnePerTenant(t *testing.T) {
	block := make(chan struct{})
	svc := newService(t, &fakeRunner{block: block}, 100)
	ctx := context.Background()
	urls := []string{"http://a"}

	first, err := svc.StartRun(ctx, "acme", 0, urls)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	waitStatus(t, svc, "acme", first.ID, healthrun.StatusRunning)

	if _, err := svc.StartRun(ctx, "acme", 0, urls); !errors.Is(err, healthrun.ErrRunInProgress) {
		t.Errorf("second for same tenant: err = %v, want ErrRunInProgress", err)
	}
	if _, err := svc.StartRun(ctx, "globex", 0, urls); err != nil {
		t.Errorf("other tenant: err = %v, want nil", err)
	}

	close(block) // let the runs finish
	waitStatus(t, svc, "acme", first.ID, healthrun.StatusDone)

	if _, err := svc.StartRun(ctx, "acme", 0, urls); err != nil {
		t.Errorf("after completion: err = %v, want nil (slot released)", err)
	}
}

func TestShutdown_CancelsInFlightRuns(t *testing.T) {
	svc := healthrun.NewService(memory.NewRunStore(), &fakeRunner{block: make(chan struct{})}, 100, zap.NewNop())
	run, _ := svc.StartRun(context.Background(), "acme", 0, []string{"http://a"})
	waitStatus(t, svc, "acme", run.ID, healthrun.StatusRunning)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := svc.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	got, _ := svc.GetRun(context.Background(), "acme", run.ID)
	if got.Status != healthrun.StatusFailed || got.Error == "" {
		t.Errorf("after shutdown run = %+v, want failed with error", got)
	}
}

func TestGetRun_TenantScoped(t *testing.T) {
	svc := newService(t, &fakeRunner{}, 100)
	run, _ := svc.StartRun(context.Background(), "acme", 0, []string{"http://a"})

	if _, err := svc.GetRun(context.Background(), "globex", run.ID); !errors.Is(err, healthrun.ErrNotFound) {
		t.Errorf("cross-tenant GetRun err = %v, want ErrNotFound", err)
	}
}
