// Package healthrun is the domain layer: it owns what a health-check run IS
// and the rules around starting one. It drives the engine and persists through
// a Store, but knows nothing about HTTP or any concrete database.
//
// The interfaces this package needs (Store, Runner) are declared HERE, by the
// consumer, not by the packages that implement them. That is the Go inversion
// of C#'s "define IFoo next to Foo": the domain says what it needs, and
// store/memory, store/postgres, and engine simply happen to satisfy it. It
// keeps the dependency arrows pointing inward and lets tests substitute fakes
// with no mocking framework.
package healthrun

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/hemantksingh/go-starter/internal/engine"
)

// Status is the lifecycle of a run. A typed string (not an int enum) so it
// serialises readably and a stray value is obvious in logs.
type Status string

const (
	StatusQueued  Status = "queued"
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusFailed  Status = "failed"
)

// Run is a plain value: no mutex, no goroutine handle. It is copied freely
// across the store boundary, which is what makes concurrent reads safe.
type Run struct {
	ID         string
	TenantID   string
	Status     Status
	URLCount   int
	CreatedAt  time.Time
	StartedAt  *time.Time // nil until running
	FinishedAt *time.Time // nil until done/failed
	Summary    *engine.Summary
	Error      string // set when Status == failed
}

// Sentinel errors. Callers compare with errors.Is; the HTTP layer maps each to
// a status code. Exported errors ARE the domain's error contract.
var (
	ErrNotFound      = errors.New("run not found")
	ErrRunInProgress = errors.New("a run is already in progress for this tenant")
	ErrTooManyURLs   = errors.New("too many urls")
	ErrNoURLs        = errors.New("no urls")
)

// Store persists runs. Implementations must be safe for concurrent use and
// must treat (tenantID, id) as the key: a run is never visible across tenants.
type Store interface {
	Create(ctx context.Context, r Run) error
	Get(ctx context.Context, tenantID, id string) (Run, error) // ErrNotFound on miss
	Update(ctx context.Context, r Run) error                   // ErrNotFound on miss
}

// Runner is what the service needs from the engine: feed it URLs, get a
// summary. *engine.Engine satisfies this without knowing the interface exists.
type Runner interface {
	Run(ctx context.Context, src io.Reader) (engine.Summary, error)
}

// Compile-time proof that the engine fits. If someone changes Engine.Run's
// signature, this line fails to build — here, in the consumer, where the
// contract lives.
var _ Runner = (*engine.Engine)(nil)
