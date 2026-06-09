// learn/03 — closures: functions are values.
//
// Run it:   go run ./learn/03-closures
//
// THE ONE IDEA: in Go a function is a value like any other — store it in a
// variable, pass it as an argument, return it from another function. An
// anonymous function written inline (`func(x int) int { ... }`) is a lambda;
// when it references variables from the enclosing scope, it's a CLOSURE.
//
// C# map: a func value is a Func<...>/Action<...> delegate; `func(){...}` is
// `x => ...`; capturing an outer variable is exactly C#'s closure capture.
//
// Why you care: closures are everywhere in Go's concurrency. `go func(){...}()`
// (a goroutine) and `g.Go(func() error {...})` (errgroup) are closures — they
// capture the variables the goroutine needs to work with.
package main

import "fmt"

// apply is a higher-order function: it TAKES a function (fn) as a parameter.
// `func(int) int` is the type "a function from int to int" — like Func<int,int>.
func apply(nums []int, fn func(int) int) []int {
	out := make([]int, len(nums))
	for i, n := range nums {
		out[i] = fn(n)
	}
	return out
}

// makeAdder RETURNS a function. The returned closure captures `base` — it keeps
// a live reference to that variable even after makeAdder has returned.
func makeAdder(base int) func(int) int {
	return func(n int) int {
		return base + n // `base` is captured from the enclosing scope
	}
}

// makeCounter shows captured state that PERSISTS and mutates across calls —
// each returned counter has its own private `count`.
func makeCounter() func() int {
	count := 0
	return func() int {
		count++ // mutates the captured variable; survives between calls
		return count
	}
}

func main() {
	nums := []int{1, 2, 3, 4}

	// 1) An anonymous function passed inline (a lambda).
	squares := apply(nums, func(n int) int { return n * n })
	fmt.Println("squares:", squares)

	// 2) A function stored in a variable, then passed by name.
	addTen := makeAdder(10)
	fmt.Println("addTen :", apply(nums, addTen))

	// 3) Captured, mutable, private state — two independent counters.
	a, b := makeCounter(), makeCounter()
	fmt.Println("counter a:", a(), a(), a()) // 1 2 3
	fmt.Println("counter b:", b())           // 1 — b has its own count
}
