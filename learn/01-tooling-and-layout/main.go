// learn/01 — tooling & layout: your first runnable Go program.
//
// Run it:    go run ./learn/01-tooling-and-layout
// Build it:  go build -o /tmp/hello ./learn/01-tooling-and-layout && /tmp/hello
//
// THE ONE IDEA: a folder whose files say `package main` and have a `func main()`
// is one runnable program. `go run <path>` compiles AND runs it in one step.
// That's the whole ceremony — no project file, no build config.
//
// Read README.md in this folder for modules, import paths, and the repo layout.
package main

import "fmt"

func main() {
	fmt.Println("hello — you just compiled and ran a Go program")
	fmt.Println("now read README.md in this folder, then move on to 02-polymorphism")
}
