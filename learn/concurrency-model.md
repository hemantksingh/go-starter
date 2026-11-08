# Concurrency model: CSP + shared memory

Go's concurrency is **CSP** (Communicating Sequential Processes, Hoare 1978):
goroutines + channels + `select`. The motto:

> Don't communicate by sharing memory; share memory by communicating.

## Why channels need no lock

- A send/receive on an unbuffered channel is a **rendezvous**:
  the sender blocks until a receiver is ready.
- That rendezvous is also a **memory barrier**: Go guarantees the send *happens-before*
  the receive. Whatever the sender wrote is visible to the receiver.
- Sending **transfers ownership**: hand the value off, don't touch it again.
  One goroutine owns the data at a time, so there is nothing to lock.

### Unbuffered vs buffered

| `make(chan T)` — unbuffered | `make(chan T, n)` — buffered |
|---|---|
| send blocks until a receiver takes the value | send blocks only when `n` values are queued |
| whoever arrives first waits for the other | sender and receiver are decoupled by the queue |
| a hand-off **and** a sync point | a queue; a send proves nothing about the receiver |

Unbuffered is the default and the safer choice: a successful send means the receiver *has*
the value. Add a buffer when you want the sender to run ahead of the receivers. The engine
does this: its dispatcher (the goroutine sending URLs) uses `jobs := make(chan string, n)`
so it isn't throttled by the slowest worker. C# has no unbuffered channel;
`Channel.CreateBounded<T>(1)` is the closest it gets.

## What "share memory by communicating" means

Sending an `int` (lesson 05) copies it: nothing is shared. The motto only bites when you
send a **pointer, slice, or map**, because then both goroutines can reach the same memory:

```go
ch := make(chan *Report)
go func() {
    r := &Report{URL: u}
    r.Status, r.Body = fetch(u) // write
    ch <- r                     // hand off: never touch r again
}()
r := <-ch                       // main owns *r now; reads it with no lock
```

The memory is shared in the plain sense (two goroutines hold the pointer). What differs
from a mutex is **who may touch it**: the data *moves* between goroutines, and exactly one
owns it at a time. Goroutines don't work on the same data; they pass it along.

The channel enforces only part of this:

| Guaranteed by the channel | Left to you |
|---|---|
| each value is received exactly once | the sender stops using it after the send |
| the sender's writes are visible to the receiver | small values go by copy, not pointer |

`ch <- r; r.Status = 500` compiles and is a data race. Go has no Rust-style move check;
"sent means gone" is a convention, and `-race` is the tool that proves you kept it.
(Same in C#: mutating an object after writing it to a `Channel<T>` is just as wrong.)

## But Go is hybrid, not pure CSP

Goroutines *can* touch the same variable. Two unsynchronized writers = a **data race**
(the `-race` detector catches it; maps may even panic: "concurrent map writes").
Go gives you two tools. Pick per situation:

| Style | Tool | Use when |
|-------|------|----------|
| Communicate (default) | channels, `select` | moving work/results between goroutines |
| Share memory (fallback) | `sync.Mutex`, `sync/atomic` | a small piece of state many goroutines must share (e.g. a metrics map) |

## Rule of thumb

**Channel** to move data/work between goroutines.
**Mutex/atomic** to guard a small shared value.

The engine in `internal/engine/` uses both: channels to dispatch URLs to the worker pool,
a mutex+atomic `Aggregator` to collect metrics. You rebuild that in `learn/08-shared-state`.
