// Package tenant resolves who a request is for.
//
// A Tenant is resolved ONCE per request by HTTP middleware (see httpapi) and
// placed on the request context; everything downstream reads it from there.
// This package knows nothing about HTTP — it owns the Tenant type, the
// Registry that loads tenants from a file, and the context plumbing.
//
// Isolation model: database per tenant (docs/adr/0002-tenant-isolation.md).
// Phase 3 adds a `DB *pgxpool.Pool` field to Tenant, opened eagerly by the
// Registry at startup and closed by Registry.Close. Until then a Tenant is
// config only.
package tenant

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

// Tenant is the per-request unit of isolation. Values are immutable after load;
// handlers receive a pointer to the registry's single instance.
type Tenant struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	MaxURLs int    `json:"max_urls,omitempty"` // 0 = use the global ENGINE_MAX_URLS
}

// Registry holds every known tenant, loaded once at startup. It is the owner
// of tenant lifecycle: construct it in main, defer Close.
type Registry struct {
	byID map[string]*Tenant
}

// idPattern: lowercase slug, so IDs are safe in headers, paths, and (later)
// database names.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// LoadFile reads a JSON array of tenants and validates it. It fails fast on a
// malformed file, a bad ID, or a duplicate — the same policy as config.Load.
func LoadFile(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("tenant: read %s: %w", path, err)
	}
	var list []Tenant
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("tenant: parse %s: %w", path, err)
	}
	return New(list)
}

// New builds a Registry from an in-memory list. Exposed so tests don't need a
// file on disk.
func New(list []Tenant) (*Registry, error) {
	if len(list) == 0 {
		return nil, fmt.Errorf("tenant: no tenants defined")
	}
	r := &Registry{byID: make(map[string]*Tenant, len(list))}
	for i := range list {
		t := list[i] // copy, so the pointer we keep doesn't alias the caller's slice
		if !idPattern.MatchString(t.ID) {
			return nil, fmt.Errorf("tenant: invalid id %q (want lowercase slug)", t.ID)
		}
		if t.MaxURLs < 0 {
			return nil, fmt.Errorf("tenant %q: max_urls must be >= 0", t.ID)
		}
		if _, dup := r.byID[t.ID]; dup {
			return nil, fmt.Errorf("tenant: duplicate id %q", t.ID)
		}
		r.byID[t.ID] = &t
	}
	return r, nil
}

// Lookup returns the tenant for id. The bool follows the map-lookup idiom
// (`v, ok := m[k]`) rather than returning nil-or-error for a miss.
func (r *Registry) Lookup(id string) (*Tenant, bool) {
	t, ok := r.byID[id]
	return t, ok
}

// Len reports how many tenants are registered.
func (r *Registry) Len() int { return len(r.byID) }

// Close releases per-tenant resources. A no-op until Phase 3 opens pools, but
// main defers it now so the lifecycle is already in the right place.
func (r *Registry) Close() error { return nil }

// --- context plumbing --------------------------------------------------------
//
// Request-scoped values ride on context.Context under an UNEXPORTED key type.
// The type is what makes the key collision-proof: no other package can
// construct a ctxKey, so nothing else can read or overwrite the tenant.
// This is the Go idiom for "HttpContext.Items", minus the string keys.

type ctxKey struct{}

// WithContext returns a child context carrying t.
func WithContext(ctx context.Context, t *Tenant) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

// FromContext returns the tenant placed by WithContext, or nil if none. Code
// behind the tenant middleware may assume non-nil; code that can run without
// it (e.g. /healthz) must check.
func FromContext(ctx context.Context) *Tenant {
	t, _ := ctx.Value(ctxKey{}).(*Tenant)
	return t
}
