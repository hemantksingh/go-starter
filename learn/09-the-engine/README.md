# Lesson 09 — the engine

The capstone. `main.go` here is a complete health-check engine in ~120 lines, built only from
what you learned in lessons 04–08. Run it: `go run -race ./learn/09-the-engine`.

## Read the output

```
checked 300, healthy 139, unhealthy 161
status codes: map[200:139 500:58]
```

The histogram counts (139 + 58 = 197) don't add up to 300 — the missing ~100 are requests that
**timed out** (slower than the 250ms deadline) and so have an error, not a status code. That gap
*is* lesson 07 working.

## Where each lesson shows up

| Concept | In this file | In production `internal/engine/` |
|---------|--------------|----------------------------------|
| goroutines + WaitGroup (04) | `go func(){...}` pool + `wg.Wait()` | same |
| channels (05) | `jobs := make(chan string, …)` | buffered `jobs` channel |
| worker pool (06) | `for i < workers { go … }` | same, size from config |
| context timeout (07) | `context.WithTimeout` in `check()` | same |
| shared state (08) | `agg` (atomic + mutex map) | the `Aggregator` type |

## What the real engine adds (and why)

Open [`internal/engine/`](../../internal/engine/) and compare. The production version layers on:

- **Streaming input** — reads URLs from an `io.Reader` line-by-line (`bufio.Scanner`), so 10,000
  URLs never sit in memory at once. Here we used a slice for simplicity.
- **`errgroup`** instead of a bare `WaitGroup` — so a dispatcher read error cancels the whole run.
- **Cancellable dispatch** — `select { case jobs <- url: case <-ctx.Done(): }`, so Ctrl-C aborts
  cleanly even mid-feed.
- **Tuned `http.Transport`** — raises `MaxIdleConnsPerHost` so same-host checks actually run in
  parallel.
- **Richer metrics** — latency percentiles and error classification (timeout / DNS / refused).

Same skeleton, production-hardened. You can now read `engine.go` and `aggregator.go` end to end.

## Done with the ladder

From here, the project's **Phase 2+** builds the web API, Postgres, auth, and tests around this
engine. See the roadmap in the top-level [`README.md`](../../README.md).
