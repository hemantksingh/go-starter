# go-starter — learn Go by building a distributed health-check engine

A hands-on Go starter for engineers coming from **C#/Java**. Instead of toy snippets, you build
**one real service**: it ingests a file of ~10,000 URLs, fans them out across a pool of worker
goroutines over channels, runs HTTP checks under strict timeout contexts, and aggregates the
results into a thread-safe store. Each piece exists for a reason: the worker pool caps how many checks
run at once so 10,000 URLs don't become 10,000 open connections; the per-request timeout stops one
dead host stalling the run; the store is thread-safe because every worker writes into the same
counters. Each phase layers on a new concept and stays runnable on its own.

## Stack

| Concern            | Choice                          | Why (for a C#/Java dev) |
|--------------------|---------------------------------|--------------------------|
| HTTP routing       | `chi` on `net/http`             | Handlers stay stdlib types; route-scoped middleware. [ADR 0001](docs/adr/0001-http-router.md) |
| Tenancy            | Database per tenant             | Isolation by construction; tenant resolved once per request. [ADR 0002](docs/adr/0002-tenant-isolation.md) |
| DB access          | `pgx` + `sqlc`                  | Write SQL, get type-safe Go. Compile-time safety, no ORM magic |
| Dependency wiring  | Manual constructor injection    | Composition over containers — how Go apps are actually built |
| Auth               | JWT bearer (OAuth2 resource server) | The standard microservice pattern: trust an external IdP |
| Logging            | Zap (structured)                | Like Serilog: typed fields, not string interpolation |

## Roadmap

- **Phase 0 — Foundation** ✅ config, Zap logger, Makefile.
- **Phase 1 — Concurrency engine** ✅ worker pool, channels, context timeouts, thread-safe aggregation.
- **Phase 2 — Web API + middleware** ⏳ chi, middleware chain, graceful shutdown.
- **Phase 3 — Postgres** ⏳ pgx pool, sqlc, migrations, repository interface.
- **Phase 4 — Auth + OpenAPI** ⏳ JWT/JWKS middleware, Swagger UI.
- **Phase 5 — Testing** ⏳ table-driven tests, `httptest`, mocks, race detector.

## New to Go? Start with the learning track

[`learn/`](learn/) is a ladder of tiny, runnable, one-idea-at-a-time lessons that build up to the
real engine — written for a C#/.NET background. See [`learn/README.md`](learn/README.md).

| # | Lesson | # | Lesson |
|---|--------|---|--------|
| 01 | tooling & layout | 05 | channels |
| 02 | polymorphism | 06 | worker pool |
| 03 | closures | 07 | context (timeouts) |
| 04 | goroutines | 08 | shared state (mutex/atomic) |

Run any lesson: `make lesson L=01`, or directly with `go run ./learn/01-tooling-and-layout`.

## Quick start (Phase 1, no infrastructure needed)

```bash
make demo
# or, step by step:
go run ./cmd/testserver &                              # offline mock targets on :8088
go run ./cmd/gen-urls -n 10000 -out testdata/urls.txt  # generate the URL file
go run -race ./cmd/worker -file testdata/urls.txt -workers 50 -timeout 3s
```

You'll get a summary like:

```
┌─ Health-check summary ────────────────────────────
│ total checked : 10000 in 3.7s
│ throughput    : ~2700 req/s
│ healthy       : 7962
│ unhealthy     : 2038
├─ latency ─────────────────────────────────────────
│ p50/p95/p99   : 0s / 180ms / 278ms
├─ status codes / errors ───────────────────────────
│ 200: 7962  404: 566  500: 826  timeout/refused/dns: ...
```

Try `-workers 200` vs `-workers 5` and watch throughput change. Run with `-race` (the default)
to confirm the shared aggregator has no data races.

## C# / Java → Go cheat sheet

### Concurrency (the headline feature)

| You know…                         | In Go…                                       | Notes |
|-----------------------------------|----------------------------------------------|-------|
| `Task` / `Thread`                 | **goroutine** (`go f()`)                     | Cheap — millions are fine. No handle returned; coordinate via channels |
| `await someTask`                  | receive from a **channel** (`v := <-ch`)     | There is no `async`/`await` keyword; blocking a goroutine is fine |
| `BlockingCollection<T>`           | **channel** (`make(chan T, n)`)              | `close(ch)` signals "no more"; `range ch` drains until closed |
| `CancellationToken` (+ deadline)  | **`context.Context`** (`context.WithTimeout`)| Pass `ctx` as the first arg everywhere; check `<-ctx.Done()` |
| `Task.WhenAll` + first-error stop | **`errgroup.Group`**                         | First non-nil error cancels the shared ctx |
| `CountdownEvent`                  | **`sync.WaitGroup`**                         | `Add`/`Done`/`Wait` |
| `lock (obj)` / `synchronized`     | **`sync.Mutex`** (`mu.Lock(); defer mu.Unlock()`) | `defer` is your `finally` for releasing it |
| `Interlocked` / `AtomicLong`      | **`sync/atomic`** (`atomic.Int64`)           | Lock-free integer counters |
| `await Task.WhenAny(...)`         | **`select { case <-a: ... case <-b: ... }`** | Wait on multiple channel ops at once |

> **Idiom:** "Don't communicate by sharing memory; share memory by communicating." The aggregator
> here uses a mutex because you asked to see a thread-safe map — but the equally-valid Go approach
> is to funnel results down one channel to a single owner goroutine that needs no lock.

### Errors, structure, lifecycle

| You know…                          | In Go…                                          |
|------------------------------------|-------------------------------------------------|
| `throw` / `try`/`catch`            | return `error` values; `if err != nil { return err }` |
| exception wrapping / `InnerException` | `fmt.Errorf("doing X: %w", err)` + `errors.Is/As` |
| `try`/`finally`                    | `defer` (runs on function exit, LIFO)           |
| `IConfiguration` → `IOptions<T>`   | a plain `Config` struct built in `config.Load()` |
| DI container (Spring/.NET)         | constructors wired by hand in `cmd/*/main.go`   |
| interface declared with impl       | interface declared by the **consumer**; impl satisfies it implicitly (structural typing) |
| `IDisposable` / `using`            | `defer thing.Close()` right after you acquire it |

## Layout

```
cmd/
  worker/       the engine CLI (Phase 1)
  gen-urls/     generates testdata/urls.txt
  testserver/   offline mock target (also your first net/http server)
internal/
  config/       typed env config
  platform/
    logger/     Zap setup
  engine/       ★ concurrency core: dispatcher, worker pool, aggregator
```

See `CLAUDE.md` for the architecture/contract details and the full command reference.
