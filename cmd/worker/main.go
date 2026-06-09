// Command worker is the standalone CLI for the health-check engine. It needs no
// database and no web server — just a file of URLs — so it's the fastest way to
// see Go's concurrency model in action.
//
//	go run -race ./cmd/worker -file testdata/urls.txt -workers 50 -timeout 3s
//
// The -race flag turns on the data-race detector. Run it that way at least once
// to convince yourself the Aggregator is actually safe under contention.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/hemantksingh/go-starter/internal/config"
	"github.com/hemantksingh/go-starter/internal/engine"
	"github.com/hemantksingh/go-starter/internal/platform/logger"
	"go.uber.org/zap"
)

func main() {
	// In Go, main() returns nothing. The idiom for "fail with a non-zero exit
	// code" is to push real logic into run() that returns an error, then
	// translate that here. Keeps main tiny and testable.
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		file    = flag.String("file", "testdata/urls.txt", "path to newline-delimited URL file")
		workers = flag.Int("workers", 0, "worker goroutines (0 = use config/env default)")
		timeout = flag.Duration("timeout", 0, "per-request timeout (0 = use config/env default)")
	)
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// CLI flags override env-derived config when provided.
	if *workers > 0 {
		cfg.Engine.Workers = *workers
	}
	if *timeout > 0 {
		cfg.Engine.Timeout = *timeout
	}

	log, err := logger.New(cfg.Env)
	if err != nil {
		return err
	}
	defer log.Sync() //nolint:errcheck // flush buffered logs on exit

	f, err := os.Open(*file)
	if err != nil {
		return fmt.Errorf("open url file: %w", err)
	}
	defer f.Close()

	// Tie the whole run to OS signals: Ctrl-C (SIGINT) or SIGTERM cancels the
	// context, which propagates to every worker and aborts in-flight requests.
	// This is how you get graceful cancellation across goroutines.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	eng := engine.New(cfg.Engine, log)

	log.Info("starting health-check run",
		zap.Int("workers", cfg.Engine.Workers),
		zap.Duration("timeout", cfg.Engine.Timeout),
		zap.String("file", *file),
	)

	started := time.Now()
	summary, err := eng.Run(ctx, f)
	elapsed := time.Since(started)
	if err != nil {
		return fmt.Errorf("engine run: %w", err)
	}

	printSummary(summary, elapsed)
	return nil
}

func printSummary(s engine.Summary, elapsed time.Duration) {
	fmt.Println()
	fmt.Println("┌─ Health-check summary ────────────────────────────")
	fmt.Printf("│ total checked : %d in %s\n", s.Total, elapsed.Round(time.Millisecond))
	if elapsed > 0 {
		fmt.Printf("│ throughput    : %.0f req/s\n", float64(s.Total)/elapsed.Seconds())
	}
	fmt.Printf("│ healthy       : %d\n", s.Success)
	fmt.Printf("│ unhealthy     : %d\n", s.Failed)
	fmt.Println("├─ latency ─────────────────────────────────────────")
	fmt.Printf("│ min/mean/max  : %s / %s / %s\n", rnd(s.Min), rnd(s.Mean), rnd(s.Max))
	fmt.Printf("│ p50/p95/p99   : %s / %s / %s\n", rnd(s.P50), rnd(s.P95), rnd(s.P99))

	if len(s.StatusHist) > 0 {
		fmt.Println("├─ status codes ────────────────────────────────────")
		for _, code := range sortedKeys(s.StatusHist) {
			fmt.Printf("│ %d : %d\n", code, s.StatusHist[code])
		}
	}
	if len(s.ErrHist) > 0 {
		fmt.Println("├─ errors ──────────────────────────────────────────")
		for label, n := range s.ErrHist {
			fmt.Printf("│ %-18s : %d\n", label, n)
		}
	}
	fmt.Println("└───────────────────────────────────────────────────")
}

func sortedKeys(m map[int]int) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func rnd(d time.Duration) time.Duration { return d.Round(time.Millisecond) }
