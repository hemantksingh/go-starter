package httpapi

import (
	"bufio"
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/hemantksingh/go-starter/internal/healthrun"
	"github.com/hemantksingh/go-starter/internal/tenant"
)

// RunService is what the handlers need from the domain. Declared here, by the
// consumer; *healthrun.Service satisfies it.
type RunService interface {
	StartRun(ctx context.Context, tenantID string, tenantMaxURLs int, urls []string) (healthrun.Run, error)
	GetRun(ctx context.Context, tenantID, id string) (healthrun.Run, error)
}

var _ RunService = (*healthrun.Service)(nil)

type handlers struct {
	runs         RunService
	maxBodyBytes int64
}

// createRun: POST /runs
//
// Body is text/plain, one URL per line (blank lines and # comments ignored),
// the same format the CLI reads from a file. The body is read fully before the
// run starts because the run outlives this request. Memory is bounded by the
// byte cap; the URL cap is a domain rule the service enforces.
func (h *handlers) createRun(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context()) // non-nil: the Tenant middleware guarantees it

	// MaxBytesReader stops reading at the cap and makes the next Read fail
	// with *http.MaxBytesError. It also closes the connection so a client
	// can't keep streaming.
	body := http.MaxBytesReader(w, r.Body, h.maxBodyBytes)
	var urls []string
	sc := bufio.NewScanner(body)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		urls = append(urls, line)
	}
	if err := sc.Err(); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "body_too_large", "request body exceeds limit")
			return
		}
		writeError(w, http.StatusBadRequest, "bad_body", "could not read request body")
		return
	}

	run, err := h.runs.StartRun(r.Context(), t.ID, t.MaxURLs, urls)
	if err != nil {
		switch {
		case errors.Is(err, healthrun.ErrNoURLs):
			writeError(w, http.StatusBadRequest, "no_urls", "body contained no urls")
		case errors.Is(err, healthrun.ErrTooManyURLs):
			writeError(w, http.StatusRequestEntityTooLarge, "too_many_urls", err.Error())
		case errors.Is(err, healthrun.ErrRunInProgress):
			writeError(w, http.StatusConflict, "run_in_progress", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "could not start run")
		}
		return
	}

	w.Header().Set("Location", "/runs/"+run.ID)
	writeJSON(w, http.StatusAccepted, toRunResponse(run))
}

// getRun: GET /runs/{id}
func (h *handlers) getRun(w http.ResponseWriter, r *http.Request) {
	t := tenant.FromContext(r.Context())
	id := chi.URLParam(r, "id")

	run, err := h.runs.GetRun(r.Context(), t.ID, id)
	if err != nil {
		if errors.Is(err, healthrun.ErrNotFound) {
			writeError(w, http.StatusNotFound, "run_not_found", "no such run")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "could not load run")
		return
	}
	writeJSON(w, http.StatusOK, toRunResponse(run))
}

// healthz: GET /healthz — liveness only. Readiness (DB reachable) is Phase 3.
func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
