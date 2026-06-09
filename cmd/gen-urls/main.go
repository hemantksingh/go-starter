// Command gen-urls writes a file of sample URLs for the worker to chew on.
//
// By default it targets the local mock server (cmd/testserver) so the whole
// demo runs offline and reproducibly — no hammering real websites. It mixes
// fast, slow, and error responses, plus a few unreachable hosts, so the
// aggregated metrics show healthy/unhealthy/timeout/dns buckets.
//
//	go run ./cmd/gen-urls -n 10000 -out testdata/urls.txt
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		n    = flag.Int("n", 10000, "number of URLs to generate")
		base = flag.String("base", "http://127.0.0.1:8088", "base URL of the mock target server")
		out  = flag.String("out", "testdata/urls.txt", "output file path")
	)
	flag.Parse()

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	// Weighted menu of endpoint shapes. The mock server understands /status/N
	// and /delay/ms; the bad hosts exercise the error-handling paths.
	r := rand.New(rand.NewSource(42)) // fixed seed => reproducible file
	for i := 0; i < *n; i++ {
		fmt.Fprintln(w, pick(r, *base))
	}

	fmt.Printf("wrote %d URLs to %s (targets %s)\n", *n, *out, *base)
	return nil
}

func pick(r *rand.Rand, base string) string {
	switch n := r.Intn(100); {
	case n < 70: // 70% healthy and fast
		return base + "/status/200"
	case n < 80: // 10% slow but healthy (small delay)
		return fmt.Sprintf("%s/delay/%d", base, 50+r.Intn(250))
	case n < 88: // 8% server errors
		return base + "/status/500"
	case n < 94: // 6% not found
		return base + "/status/404"
	case n < 98: // 4% unreachable port -> connection refused
		return "http://127.0.0.1:9/dead"
	default: // 2% bad DNS -> name resolution failure
		return "http://no-such-host.invalid/"
	}
}
