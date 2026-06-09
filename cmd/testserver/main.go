// Command testserver is a tiny httpbin-style mock used as the target for the
// health-check demo, so everything runs offline. It also doubles as your first
// look at Go's net/http server before we add chi in Phase 2.
//
//	go run ./cmd/testserver            # listens on :8088
//
// Endpoints:
//
//	GET /status/{code}   -> responds with that HTTP status
//	GET /delay/{ms}      -> sleeps {ms} milliseconds, then 200 (honors ctx)
//	GET /                -> 200 ok
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

func main() {
	addr := flag.String("addr", ":8088", "listen address")
	flag.Parse()

	mux := http.NewServeMux()

	// Go 1.22+ ServeMux supports method+pattern routing with path wildcards,
	// so we get typed path params without a third-party router here.
	mux.HandleFunc("GET /status/{code}", func(w http.ResponseWriter, r *http.Request) {
		code, err := strconv.Atoi(r.PathValue("code"))
		if err != nil || code < 100 || code > 599 {
			code = http.StatusBadRequest
		}
		w.WriteHeader(code)
		fmt.Fprintf(w, "status %d\n", code)
	})

	mux.HandleFunc("GET /delay/{ms}", func(w http.ResponseWriter, r *http.Request) {
		ms, _ := strconv.Atoi(r.PathValue("ms"))
		select {
		case <-time.After(time.Duration(ms) * time.Millisecond):
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "slept %dms\n", ms)
		case <-r.Context().Done():
			// The client (our engine) hit its timeout and cancelled; stop work.
			return
		}
	})

	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "ok")
	})

	fmt.Printf("mock target server listening on %s\n", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, "testserver:", err)
		os.Exit(1)
	}
}
