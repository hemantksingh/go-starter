# ADR 0001: HTTP router is chi on net/http

**Status:** accepted, 2026-11-13

## Context

Phase 2 adds a web API. The project is pedagogical: an engineer from C#/.NET learning how Go
builds HTTP services. The middleware design calls for three scopes of wrapping: observability on
every route, tenant resolution on every API route, and (from Phase 4) a principal check on write
routes only, attached where the route is registered.

Candidates:

- **`net/http.ServeMux`** (stdlib). Since Go 1.22 it routes by method and path wildcards. It has
  no route groups or route-scoped middleware; each handler would be wrapped by hand.
- **chi.** A router over `net/http`. Handlers are `http.HandlerFunc`, middleware is
  `func(http.Handler) http.Handler`. Adds route groups (`r.Group`, `r.With(...)`), URL params,
  and a small middleware set (request ID, real IP, recoverer, timeout).
- **gin.** A framework. Handlers take `*gin.Context`, which wraps the request, response, and a
  request-scoped value map; middleware is gin-specific (`c.Next()`). Bundles JSON binding,
  validation, and rendering.
- **echo.** Similar shape to gin: its own context and handler types.

## Decision

Use **chi**.

## Rationale

- **Handlers stay stdlib types.** Everything written against chi is a plain `http.Handler`. It
  works with `httptest`, with any other `net/http` middleware, and would survive removing chi.
  gin and echo put their own context type between the code and `net/http`, which hides the model
  this project exists to teach.
- **Route-scoped middleware is declared at registration.** `r.With(principal).Post("/runs", h)`
  is exactly the wiring shape the design asks for. With the stdlib mux this is a hand-written
  wrapper per route.
- **Request-scoped values use `context.Context`.** chi leaves `r.Context()` as the only
  mechanism, which matches the typed context-key idiom used for the tenant. gin's `c.Set/c.Get`
  is a string-keyed map that works against it.
- **Familiarity is the trap, not the goal.** `*gin.Context` resembles ASP.NET's `HttpContext`,
  which is the reason to avoid it here.

## Consequences

- One small dependency (`github.com/go-chi/chi/v5`). No transitive framework.
- Response rendering, JSON decoding, and validation are written by hand in `internal/httpapi`.
  That is deliberate: it is a few lines each and shows what a framework would otherwise hide.
- Performance differences between the candidates are measurable in micro-benchmarks and
  irrelevant at this service's request rates. Not a factor.
- `cmd/testserver` keeps using the stdlib mux. It is a mock, and seeing both side by side is
  useful.
