package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/hemantksingh/go-starter/internal/tenant"
	"go.uber.org/zap"
)

// A middleware in Go is a function that takes a handler and returns a handler.
// That's the whole contract — no interface, no base class — which is why
// middlewares from any library compose with any router:
//
//	func(next http.Handler) http.Handler
//
// Order matters and is set in router.go: Observability is OUTERMOST so it sees
// every request including ones the tenant middleware rejects, and so its
// deferred log line runs last.

// --- request-scoped state ----------------------------------------------------

type ctxKey int

const (
	keyRequestID ctxKey = iota
	keyRequestInfo
)

// requestInfo is filled in by INNER middlewares for the OUTER one to log.
// Context values only flow inward (a child ctx can't change its parent), so
// the outer middleware plants a pointer and the tenant middleware writes
// through it. It is per-request and only touched by this request's goroutine,
// so no lock.
type requestInfo struct {
	Tenant string
}

// RequestIDFromContext returns the ID the observability middleware assigned.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(keyRequestID).(string)
	return id
}

func infoFromContext(ctx context.Context) *requestInfo {
	info, _ := ctx.Value(keyRequestInfo).(*requestInfo)
	return info
}

// --- observability -----------------------------------------------------------

// Observability is the outermost wrapper. Per request it:
//   - assigns a request ID (honouring an incoming X-Request-ID) and echoes it;
//   - recovers a panic into a 500 instead of killing the server;
//   - writes one structured access-log line on the way out.
func Observability(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			reqID := r.Header.Get("X-Request-ID")
			if reqID == "" {
				reqID = newRequestID()
			}
			w.Header().Set("X-Request-ID", reqID)

			info := &requestInfo{}
			ctx := context.WithValue(r.Context(), keyRequestID, reqID)
			ctx = context.WithValue(ctx, keyRequestInfo, info)
			r = r.WithContext(ctx)

			// net/http's ResponseWriter doesn't expose the status after the
			// fact; chi's wrapper records it. (Hand-rolling it is ~15 lines of
			// embedding + an overridden WriteHeader.)
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered",
						zap.Any("panic", rec),
						zap.String("request_id", reqID),
						zap.ByteString("stack", debug.Stack()))
					if ww.Status() == 0 { // nothing written yet: we can still answer
						writeError(ww, http.StatusInternalServerError, "internal_error", "internal server error")
					}
				}
				log.Info("request",
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.Int("status", ww.Status()),
					zap.Int("bytes", ww.BytesWritten()),
					zap.Duration("duration", time.Since(start)),
					zap.String("request_id", reqID),
					zap.String("tenant", info.Tenant), // "" if rejected before resolution
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// --- tenant ------------------------------------------------------------------

// Tenant resolves the tenant ONCE per request from the X-Tenant-ID header and
// puts it on the context for every handler below. Applied to the API group
// only; /healthz sits outside it so a load balancer needs no header.
func Tenant(reg *tenant.Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Tenant-ID")
			if id == "" {
				writeError(w, http.StatusBadRequest, "missing_tenant", "X-Tenant-ID header is required")
				return
			}
			t, ok := reg.Lookup(id)
			if !ok {
				writeError(w, http.StatusNotFound, "unknown_tenant", "no such tenant")
				return
			}
			if info := infoFromContext(r.Context()); info != nil {
				info.Tenant = t.ID // visible to the outer access log
			}
			next.ServeHTTP(w, r.WithContext(tenant.WithContext(r.Context(), t)))
		})
	}
}
