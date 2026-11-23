// Package httpapi is the transport layer: routing, middleware, and the JSON
// shapes on the wire. It translates HTTP into calls on the domain (healthrun)
// and domain errors back into status codes. Nothing below it imports it.
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/hemantksingh/go-starter/internal/healthrun"
	"github.com/hemantksingh/go-starter/internal/tenant"
	"go.uber.org/zap"
)

// Deps is everything the router needs, handed in by the composition root.
// Plain struct, plain constructor: manual DI.
type Deps struct {
	Log          *zap.Logger
	Tenants      *tenant.Registry
	Runs         *healthrun.Service
	MaxBodyBytes int64
}

// New builds the http.Handler for the whole API. The middleware order is the
// design: observability outermost, tenant resolution on the API group only,
// and (Phase 4) a principal check attached to write routes at registration.
func New(d Deps) http.Handler {
	h := &handlers{runs: d.Runs, maxBodyBytes: d.MaxBodyBytes}

	r := chi.NewRouter()
	r.Use(Observability(d.Log))

	// chi's defaults for unmatched routes are plain-text; keep the envelope.
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such route")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})

	// Outside the tenant group: no header needed.
	r.Get("/healthz", healthz)

	// The API proper: everything here runs behind tenant resolution.
	r.Group(func(r chi.Router) {
		r.Use(Tenant(d.Tenants))
		r.Post("/runs", h.createRun) // Phase 4: r.With(Principal(...)).Post(...)
		r.Get("/runs/{id}", h.getRun)
	})

	return r
}
