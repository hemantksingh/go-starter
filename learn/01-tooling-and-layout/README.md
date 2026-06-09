# Lesson 01 — tooling & project layout

Orientation for developers from **C#/.NET**. Read this once and the repo stops looking weird.
Run the program here first: `go run ./learn/01-tooling-and-layout`.

## 1. No `src/`, almost no enforced layout

The folder containing **`go.mod`** is the project root; code lives directly under it. No `src/`,
no per-project `.csproj`.

The compiler enforces **one** layout rule:

- **`internal/`** — packages here can only be imported by code under that folder's parent.
  Compiler-enforced privacy — the closest thing to `internal` in C#.

Everything else (`cmd/`, `pkg/`) is convention. Lay the project out around your **domain**:
carve by *what things are*, not by technical layer. Go forbids **circular imports**, so
dependencies are forced to flow one way — that keeps the domain at the center for free.

| Folder | Meaning | C# analogy |
|--------|---------|------------|
| `cmd/<name>/` | a runnable program (`package main` + `func main()`) | a console app with `Main()` |
| `internal/...` | library code, importable only within this module | a class library marked `internal` |

## 2. Imports look like URLs

`go.mod` names the module — by convention, the repo URL:

```
module github.com/hemantksingh/go-starter
```

Every import is **that name + the folder path from the root**:

```
github.com/hemantksingh/go-starter / internal/engine
└──────────── module name ─────────┘ └── dir path ──┘
```

So **the import path *is* the directory structure**. The URL-looking prefix exists because an
import path doubles as a **download address** (`go get github.com/...` fetches from there — there
is no central registry like NuGet). For *your own* code nothing hits the network: `go.mod`
declares this module's name, so imports with that prefix resolve to local folders.

| | Unique naming | Mirrors dirs? |
|---|---|---|
| **C#** | namespace / assembly: `Example.Engine` | loosely; not enforced |
| **Go** | repo URL: `github.com/you/proj/internal/engine` | yes — path from module root |

The **package name** (`package engine`, used as `engine.New()`) usually matches the last folder
segment, but isn't required to.

## 3. Everyday commands

```bash
go run ./cmd/worker     # compile AND run (your dev loop)
go build ./...          # compile everything; "./..." = this dir + all subdirs
go test ./...           # run tests (files named *_test.go)
go mod tidy             # add missing / drop unused dependencies
go fmt ./...            # auto-format (one canonical style; no debates)
```

Two bits of syntax:
- **`./cmd/worker`** — a *path* to a folder; leading `./` means "local," not a package to download.
- **`./...`** — "this directory and everything beneath it."

➡️ Next: [`02-polymorphism`](../02-polymorphism) — one type, many implementations.
