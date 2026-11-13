package healthrun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Service starts and reports on runs. One instance per process, used by
// pointer (it holds a mutex).
//
// Runs execute in the background. They must NOT run on the HTTP request's
// context — that is cancelled the moment the handler returns, long before a
// 10k-URL run finishes. The service owns a base context created at
// construction and cancelled by Shutdown; every run derives from that.
type Service struct {
	store   Store
	runner  Runner
	log     *zap.Logger
	maxURLs int // global cap; a tenant's MaxURLs overrides when > 0

	// One run in flight per tenant. This is a domain rule, so it lives here,
	// not in the store or the handler. The mutex makes check-then-claim atomic.
	mu     sync.Mutex
	active map[string]struct{}

	baseCtx context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup // tracks run goroutines so Shutdown can wait
}

// NewService wires the service. maxURLs is the global ENGINE_MAX_URLS.
func NewService(store Store, runner Runner, maxURLs int, log *zap.Logger) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		store:   store,
		runner:  runner,
		log:     log,
		maxURLs: maxURLs,
		active:  make(map[string]struct{}),
		baseCtx: ctx,
		cancel:  cancel,
	}
}

// StartRun validates, claims the tenant's slot, records the run as queued, and
// kicks off the engine in the background. It returns as soon as the run is
// persisted; the caller polls GetRun for progress.
//
// tenantMaxURLs is the tenant's override (0 = none). It is passed as a value,
// not read from ctx, so this package stays ignorant of the tenant package and
// of how the tenant was resolved.
func (s *Service) StartRun(ctx context.Context, tenantID string, tenantMaxURLs int, urls []string) (Run, error) {
	if len(urls) == 0 {
		return Run{}, ErrNoURLs
	}
	limit := s.maxURLs
	if tenantMaxURLs > 0 {
		limit = tenantMaxURLs
	}
	if len(urls) > limit {
		return Run{}, fmt.Errorf("%w: %d > limit %d", ErrTooManyURLs, len(urls), limit)
	}

	// Claim the per-tenant slot. Lock, check, insert, unlock — one critical
	// section, so two simultaneous requests can't both see "free".
	s.mu.Lock()
	if _, busy := s.active[tenantID]; busy {
		s.mu.Unlock()
		return Run{}, ErrRunInProgress
	}
	s.active[tenantID] = struct{}{}
	s.mu.Unlock()

	run := Run{
		ID:        newID(),
		TenantID:  tenantID,
		Status:    StatusQueued,
		URLCount:  len(urls),
		CreatedAt: time.Now(),
	}
	if err := s.store.Create(ctx, run); err != nil {
		s.release(tenantID) // claimed but never started: give the slot back
		return Run{}, fmt.Errorf("create run: %w", err)
	}

	s.wg.Add(1)
	go s.execute(run, urls)
	return run, nil
}

// execute is the background half of StartRun. Every state change writes a
// fresh Run VALUE to the store; nothing here mutates memory a concurrent GET
// could be reading. The store's lock is the only synchronisation needed.
func (s *Service) execute(run Run, urls []string) {
	defer s.wg.Done()
	defer s.release(run.TenantID) // runs after the final store write below

	log := s.log.With(zap.String("run_id", run.ID), zap.String("tenant", run.TenantID))

	now := time.Now()
	run.Status = StatusRunning
	run.StartedAt = &now
	if err := s.store.Update(s.baseCtx, run); err != nil {
		log.Error("mark running", zap.Error(err))
		return
	}

	// The engine streams from an io.Reader; we hand it the buffered URLs.
	summary, err := s.runner.Run(s.baseCtx, strings.NewReader(strings.Join(urls, "\n")))

	done := time.Now()
	run.FinishedAt = &done
	if err != nil {
		run.Status = StatusFailed
		run.Error = err.Error()
		log.Warn("run failed", zap.Error(err), zap.Duration("took", done.Sub(now)))
	} else {
		run.Status = StatusDone
		run.Summary = &summary
		log.Info("run done",
			zap.Int64("total", summary.Total),
			zap.Int64("failed", summary.Failed),
			zap.Duration("took", done.Sub(now)))
	}
	// Use the base ctx, not a cancelled one: on shutdown we still want the
	// final state recorded. context.WithoutCancel keeps values, drops cancel.
	if err := s.store.Update(context.WithoutCancel(s.baseCtx), run); err != nil {
		log.Error("record final state", zap.Error(err))
	}
}

func (s *Service) release(tenantID string) {
	s.mu.Lock()
	delete(s.active, tenantID)
	s.mu.Unlock()
}

// GetRun returns a run scoped to its tenant. A run belonging to another tenant
// is ErrNotFound, not forbidden: IDs must leak nothing across the boundary.
func (s *Service) GetRun(ctx context.Context, tenantID, id string) (Run, error) {
	return s.store.Get(ctx, tenantID, id)
}

// Shutdown cancels every in-flight run and waits for their goroutines to
// record a final state, or for ctx to expire. Call after the HTTP server has
// stopped accepting requests.
func (s *Service) Shutdown(ctx context.Context) error {
	s.cancel()
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("healthrun: shutdown: %w", ctx.Err())
	}
}

// newID is 128 random bits as hex. No UUID dependency: crypto/rand is in the
// stdlib and the format is opaque to callers anyway.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error()) // the OS RNG failing is not recoverable
	}
	return hex.EncodeToString(b[:])
}
