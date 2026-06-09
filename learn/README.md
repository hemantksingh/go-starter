# learn/ — Go from zero, one runnable step at a time

A ladder of tiny, self-contained programs for a developer coming from **C#/.NET**. Each lesson
adds exactly one idea and runs on its own. Walk them in order; by the end you can read the real
`internal/engine/` code and see it's just these pieces wired together.

Run any lesson with `go run ./learn/<folder>` (some note a `-race` variant).

## The ladder

| # | Lesson | Idea | C# touchstone |
|---|--------|------|---------------|
| 01 | [tooling-and-layout](01-tooling-and-layout) | go run/build/test, modules, import paths, repo layout | projects, namespaces, NuGet |
| 02 | [polymorphism](02-polymorphism) | many implementations via implicit interfaces | interfaces (but no `implements`) |
| 03 | [closures](03-closures) | functions as values; closures capture state | lambdas, `Func`/`Action` |
| 04 | [goroutines](04-goroutines) | start concurrent work, wait for it | `Task`, `CountdownEvent` |
| 05 | [channels](05-channels) | goroutines hand results back | `BlockingCollection<T>` |
| 06 | [worker-pool](06-worker-pool) | a fixed pool draining one queue | `Parallel.ForEach(MaxDegreeOfParallelism)` |
| 07 | [context](07-context) | timeouts & cancellation | `CancellationToken` |
| 08 | [shared-state](08-shared-state) | combine results safely | `lock`, `Interlocked` |
| 09 | the-engine *(soon)* | read a file + wire it all together | — |

## Concept note

[`concurrency-model.md`](concurrency-model.md) — CSP vs shared memory, and when to reach for a
channel vs a mutex. Read it alongside lessons 05–08.

## External resources

- [**Go by Example**](https://gobyexample.com/) — short, runnable snippets for nearly every
  language feature. The ideal companion to these lessons: look up `goroutines`, `channels`,
  `interfaces`, `closures`, `context`, `mutexes` for a second angle on each idea.
- [A Tour of Go](https://go.dev/tour/) — the official interactive intro.
- [Effective Go](https://go.dev/doc/effective_go) — idioms and conventions.
