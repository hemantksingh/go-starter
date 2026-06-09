// learn/02 — polymorphism: one type, many implementations (via interfaces).
//
// Run it:   go run ./learn/02-polymorphism
//
// THE ONE IDEA: an interface is a set of method signatures. Any type that has
// those methods SATISFIES the interface — automatically. There is no
// `class Foo : IBar` and no `implements` keyword. If it has the methods, it fits.
// This is "structural typing" (a.k.a. duck typing, checked at compile time).
//
// C# map: like an interface + Func dispatch, but you never DECLARE that a type
// implements it. You just write the methods; the compiler verifies the fit at
// the point of use. This lets you make types you don't own satisfy YOUR
// interface, and keeps packages decoupled.
//
// THE GO IDIOM: "accept interfaces, return structs." Functions take the small
// interface they need (the behavior), not a concrete type — so callers can swap
// implementations freely (real one in prod, a fake in tests). You'll see this
// in Phase 3: the `health` package defines the Repository interface it needs,
// and the Postgres `store` satisfies it without either importing a base class.
package main

import "fmt"

// Check is the interface: anything with these two methods IS a Check.
type Check interface {
	Name() string
	Run() error
}

// --- Three unrelated structs. None says "implements Check". ---

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
