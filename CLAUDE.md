# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A pedagogical Go project for an engineer coming from C#/Java. It is built as **one coherent
application** — a multi-tenant URL health-check service — that introduces Go's idioms layer by
layer rather than as disconnected examples. Design decisions with real alternatives are recorded
as ADRs in `docs/adr/`; read those before changing the router or the tenancy model.

**Build phases incrementally; each must stay independently runnable.** When asked to add a
capability, prefer extending the current phase's slice over scaffolding everything at once.
Current state: **Phases 0–2 are implemented**: foundation, the concurrency engine, and the
multi-tenant HTTP API (`cmd/api`), each with tests. Phases 3–5 (Postgres via pgx/sqlc with one
database per tenant, JWT auth + OpenAPI, tests for the store layer) are planned but not yet
written.

## Learning track (`learn/`)

A ladder of tiny one-idea-at-a-time runnable lessons for a C#/.NET background, built incrementally
on user request. Numbered: `01-tooling-and-layout` (+ its own README), `02-polymorphism`,
`03-closures`, `04-goroutines`, `05-channels`, `06-worker-pool`, `07-context`, `08-shared-state`,
`09-the-engine` (planned). `learn/README.md` is the index; `learn/concurrency-model.md` is a
concept note. When adding/renumbering lessons, keep recap references concept-based (not "step N")
and update both READMEs + the memory note. Use **C# analogies, not Java**.

## Commands

```bash
make help              # list all targets
make build             # compile all cmds into ./bin
make api               # run the HTTP API on :8080 (race on); see cmd/api/main.go for curl examples
make demo              # start mock server, generate 10k URLs, run the engine (race on)
make test              # go test -race ./...   (race detector is always on)
make vet               # go vet ./...
make fmt               # gofmt -w .

# The engine, run manually (its standalone CLI needs no DB/web):
go run ./cmd/testserver &                                   # offline mock targets on :8088
go run ./cmd/gen-urls -n 10000 -out testdata/urls.txt       # synthesize the URL file
go run -race ./cmd/worker -file testdata/urls.txt -workers 50 -timeout 3s

# Run a single test / package:
go test -race ./internal/engine/ -run TestAggregator -v
```

`-race` is intentionally on by default for `test`, `worker`, and `demo`: this project teaches
concurrency, and the race detector is the tool that proves the shared state is actually safe.
Keep it that way when adding targets.

## Architecture

`docs/architecture.md` walks one request through every component and says why each boundary
exists; read it first if the layering looks like indirection. The dependency direction is the
central lesson and must be preserved:

```
cmd/* (composition root: the ONLY place concrete types are wired together)
  └─ httpapi ──▶ healthrun (Service) ──▶ store/* (healthrun.Store impls)
     (transport)    (domain logic)           (persistence; memory now, postgres in Phase 3)
         │               └─ drives ─▶ engine (concurrency library, satisfies healthrun.Runner)
         └─ resolves ─▶ tenant (Registry + context plumbing; owns per-tenant pools in Phase 3)
```

- **`cmd/*` is the composition root.** Dependencies are assembled with plain constructors in
  `main` — no DI container, no reflection. This is manual dependency injection by design; do not
  introduce wire/fx without discussion.
- **Interfaces are declared by the consumer, not the implementer.** `healthrun` defines `Store`
  and `Runner` (what it needs); `store/memory` and `engine` satisfy them without importing it.
  Declare an interface only where a second implementation or a test fake exists: `httpapi`
  holds `*healthrun.Service` directly. The package is `healthrun`, not `health`, to avoid
  colliding with `/healthz`.
- **Multi-tenancy is database-per-tenant** (ADR 0002). The tenant is resolved ONCE per request
  by middleware from `X-Tenant-ID` and placed on the context via `tenant.WithContext`; nothing
  below the middleware chooses a tenant. `/healthz` sits outside the tenant group.
- **Middleware order is the design**: observability outermost (request ID, Zap access log,
  recoverer), tenant resolution on the API group, and in Phase 4 a principal check attached to
  write routes at registration (`r.With(Principal).Post(...)`). Runs execute on the service's
  base context, never on `r.Context()`, which dies when the handler returns.
- **`internal/engine` is a self-contained library** with no knowledge of HTTP handlers, configs
  beyond `config.EngineConfig`, or databases. Keep it that way so it stays unit-testable in
  isolation.

### The engine (`internal/engine`) — the concurrency contract

A fan-out/fan-in pipeline. Understand this before modifying it:

- `engine.go` — `Engine.Run(ctx, io.Reader)` is the entry point. A single **dispatcher**
  goroutine streams URLs (line-by-line, never all in memory) onto a buffered `jobs` channel;
  **N worker** goroutines consume it. `golang.org/x/sync/errgroup` ties them together so the
  first error cancels the shared `ctx`. The dispatcher `close()`s `jobs` to signal completion.
- Each check uses a **per-request `context.WithTimeout`** derived from the run's `ctx` (the Go
  analog of a linked `CancellationTokenSource` with a deadline). The shared `*http.Client` has
  **no** `Timeout` — cancellation is via context so a parent cancel aborts in-flight requests.
- `aggregator.go` — `Aggregator` is the **thread-safe sink** every worker writes to concurrently.
  It deliberately shows both primitives: `sync/atomic` counters (lock-free) and a `sync.Mutex`
  guarding the histogram maps + latency slice. A nil map panics on write, so maps are initialized
  in `newAggregator()`. `Summary()` is the read-side snapshot (percentiles) taken after workers
  finish.

When touching the engine, the invariants are: don't write a shared map without holding `mu`;
always `defer cancel()` on a context you create; always drain+close `resp.Body` (keep-alive).

### Config & platform

- `internal/config` — typed `Config` struct built once from env vars (hand-rolled typed getters,
  zero deps). `Load()` returns `(Config, error)`; validation fails fast at startup. No global
  singleton — it is passed down explicitly.
- `internal/platform/logger` — Zap. `development` env → console; otherwise JSON. Callers own the
  logger and `defer logger.Sync()`.

## Conventions

- **Errors are values, returned not thrown.** Wrap with `fmt.Errorf("context: %w", err)` to
  preserve the chain for `errors.Is`/`errors.As`. `main` is the only place that translates an
  error into `os.Exit(1)`; keep `main` thin by delegating to a `run() error`.
- **Module path:** `github.com/hemantksingh/go-starter`. Application packages live under `internal/`
  (compiler-enforced privacy). Entrypoints under `cmd/<name>/main.go`.
- Toolchain is Go 1.26 (`go.mod` declares it). `cmd/testserver` uses Go 1.22+ `ServeMux`
  method+wildcard routing — fine for a mock, but Phase 2's real API uses chi.
