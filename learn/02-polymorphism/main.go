// learn/02 — polymorphism: one type, many implementations (via interfaces).
//
// Run it:   go run ./learn/02-polymorphism
//
// THE ONE IDEA: an interface is a set of method signatures. Any type that has
// those methods SATISFIES the interface — automatically. There is no
// `class Foo : IBar` and no `implements` keyword. If it has the methods, it fits.
// This is "structural typing" (a.k.a. duck typing, checked at compile time).
//
// C# map: in C# a class must say `: ICheck` to count as an ICheck. In Go there
// is no such line. The compiler checks the fit only where you USE a value as a
// Check (e.g. putting it in a []Check). Upshot: you can declare an interface in
// your package and have a type from a library you don't own satisfy it.
//
// THE GO IDIOM: "accept interfaces." A rule about parameters: ask for the
// interface, e.g. runAll(checks []Check) below, not runAll(checks []DiskCheck)
// — then any Check works, including a fake in tests. (Same instinct as taking
// IEnumerable<T> over List<T> in C#.) Phase 3 does exactly this: `health`
// declares the Repository interface it needs; the Postgres `store` satisfies it.
package main

import "fmt"

// Check is the interface: anything with these two methods IS a Check.
type Check interface {
	Name() string
	Run() error
}

// --- Three unrelated structs. None says "implements Check". ---
//
// No class keyword. In C# you put a method inside the class braces to say "this
// belongs to PingCheck". Go has no class braces, so it says the same thing with
// the RECEIVER: the `(p PingCheck)` before the method name. `p` is C#'s `this`,
// just named explicitly. The method is a member of the struct either way.
// Structs are value types (copied on assign, like C# structs); use `*PingCheck`
// for C#-class-style reference semantics.

type PingCheck struct{ Host string }

func (p PingCheck) Name() string { return "ping " + p.Host }
func (p PingCheck) Run() error   { return nil } // pretend it succeeds

type DiskCheck struct{ FreePct int }

func (d DiskCheck) Name() string { return "disk" }
func (d DiskCheck) Run() error {
	if d.FreePct < 10 {
		return fmt.Errorf("only %d%% free", d.FreePct)
	}
	return nil
}

type HTTPCheck struct{ URL string }

func (h HTTPCheck) Name() string { return "http " + h.URL }
func (h HTTPCheck) Run() error   { return fmt.Errorf("503 service unavailable") }

// runAll accepts the INTERFACE, so it works with any current or future Check
// implementation — that's the polymorphism. It never names a concrete type.
func runAll(checks []Check) {
	for _, c := range checks {
		if err := c.Run(); err != nil { // dynamic dispatch: the real type's Run runs
			fmt.Printf("  ✗ %-22s FAIL: %v\n", c.Name(), err)
			continue
		}
		fmt.Printf("  ✓ %-22s ok\n", c.Name())
	}
}

func main() {
	// A slice of Check values holding three different concrete types.
	checks := []Check{
		PingCheck{Host: "db.internal"},
		DiskCheck{FreePct: 4},
		HTTPCheck{URL: "https://api.example.com"},
	}
	runAll(checks)
}
