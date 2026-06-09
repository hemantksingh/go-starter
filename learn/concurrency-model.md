# Concurrency model: CSP + shared memory

Go's concurrency is **CSP** (Communicating Sequential Processes, Hoare 1978): goroutines +
channels + `select`. The motto:

> Don't communicate by sharing memory; share memory by communicating.

## Why channels need no lock

- A send/receive on an unbuffered channel is a **rendezvous**: the sender blocks until a receiver
  is ready.
- That rendezvous is also a **memory barrier** — Go guarantees the send *happens-before* the
  receive. Whatever the sender wrote is visible to the receiver.
- Sending **transfers ownership**: hand the value off, don't touch it again. One goroutine owns
  the data at a time → nothing to lock.

## But Go is hybrid, not pure CSP

Goroutines *can* touch the same variable. Two unsynchronized writers = a **data race** (the
`-race` detector catches it; maps may even panic: "concurrent map writes"). Go gives you two
tools — pick per situation:

| Style | Tool | Use when |
|-------|------|----------|
| Communicate (default) | channels, `select` | moving work/results between goroutines |
| Share memory (fallback) | `sync.Mutex`, `sync/atomic` | a small piece of state many goroutines must share (e.g. a metrics map) |

## Rule of thumb

**Channel** to move data/work between goroutines. **Mutex/atomic** to guard a small shared value.

The engine in `internal/engine/` uses both: channels to dispatch URLs to the worker pool, a
mutex+atomic `Aggregator` to collect metrics. You rebuild that in `learn/08-shared-state`.
