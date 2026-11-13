// Package memory is the Phase 2 healthrun.Store: a map behind a mutex. It is
// replaced by a Postgres store in Phase 3 without the service changing, and it
// doubles as the store the HTTP handler tests use.
package memory

import (
	"context"
	"sync"

	"github.com/hemantksingh/go-starter/internal/healthrun"
)

// RunStore keeps runs keyed by (tenant, id). Runs are stored and returned BY
// VALUE: the copy across the lock boundary is what makes a concurrent GET safe
// against the background goroutine writing a new state.
type RunStore struct {
	mu   sync.RWMutex
	runs map[key]healthrun.Run
}

type key struct{ tenant, id string }

// NewRunStore returns an empty store. The map is made here: a nil map panics
// on write.
func NewRunStore() *RunStore {
	return &RunStore{runs: make(map[key]healthrun.Run)}
}

// Compile-time check that this satisfies the consumer's interface.
var _ healthrun.Store = (*RunStore)(nil)

func (s *RunStore) Create(_ context.Context, r healthrun.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[key{r.TenantID, r.ID}] = r
	return nil
}

// Get is tenant-scoped: the wrong tenant gets ErrNotFound, never a hint that
// the id exists.
func (s *RunStore) Get(_ context.Context, tenantID, id string) (healthrun.Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.runs[key{tenantID, id}]
	if !ok {
		return healthrun.Run{}, healthrun.ErrNotFound
	}
	return r, nil
}

func (s *RunStore) Update(_ context.Context, r healthrun.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key{r.TenantID, r.ID}
	if _, ok := s.runs[k]; !ok {
		return healthrun.ErrNotFound
	}
	s.runs[k] = r
	return nil
}
