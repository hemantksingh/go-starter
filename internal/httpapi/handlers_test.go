package httpapi_test

// Handler tests go through the real router with the real domain service and
// memory store; only the engine is faked. That exercises the middleware
// order, the error mapping, and the DTOs exactly as a client would see them.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hemantksingh/go-starter/internal/engine"
	"github.com/hemantksingh/go-starter/internal/healthrun"
	"github.com/hemantksingh/go-starter/internal/httpapi"
	"github.com/hemantksingh/go-starter/internal/store/memory"
	"github.com/hemantksingh/go-starter/internal/tenant"
	"go.uber.org/zap"
)

type fakeRunner struct {
	block chan struct{}
}

func (f *fakeRunner) Run(ctx context.Context, src io.Reader) (engine.Summary, error) {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return engine.Summary{}, ctx.Err()
		}
	}
	data, _ := io.ReadAll(src)
	n := int64(len(strings.Fields(string(data))))
	return engine.Summary{
		Total: n, Success: n,
		StatusHist: map[int]int{200: int(n)}, ErrHist: map[string]int{},
		Min: time.Millisecond, Max: 3 * time.Millisecond, P50: 2 * time.Millisecond,
	}, nil
}

type api struct {
	srv *httptest.Server
}

func newAPI(t *testing.T, runner healthrun.Runner, maxURLs int, maxBody int64) *api {
	t.Helper()
	reg, err := tenant.New([]tenant.Tenant{
		{ID: "acme", Name: "Acme"},
		{ID: "globex", Name: "Globex", MaxURLs: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := healthrun.NewService(memory.NewRunStore(), runner, maxURLs, zap.NewNop())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = svc.Shutdown(ctx)
	})

	h := httpapi.New(httpapi.Deps{Log: zap.NewNop(), Tenants: reg, Runs: svc, MaxBodyBytes: maxBody})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &api{srv: srv}
}

// do sends a request with the tenant header (empty string = omit) and decodes
// the JSON body into out if non-nil.
func (a *api) do(t *testing.T, method, path, tenantID, body string, out any) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, a.srv.URL+path, strings.NewReader(body))
	if tenantID != "" {
		req.Header.Set("X-Tenant-ID", tenantID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
	return resp
}

type errEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type runBody struct {
	ID       string          `json:"id"`
	Tenant   string          `json:"tenant"`
	Status   string          `json:"status"`
	URLCount int             `json:"url_count"`
	Summary  *map[string]any `json:"summary"`
	Error    string          `json:"error"`
}

func (a *api) waitStatus(t *testing.T, tenantID, id, want string) runBody {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var rb runBody
		a.do(t, http.MethodGet, "/runs/"+id, tenantID, "", &rb)
		if rb.Status == want {
			return rb
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("run %s never reached %s", id, want)
	return runBody{}
}

func TestHealthz_NeedsNoTenant(t *testing.T) {
	a := newAPI(t, &fakeRunner{}, 100, 1<<20)
	resp := a.do(t, http.MethodGet, "/healthz", "", "", nil)
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("no X-Request-ID on response")
	}
}

func TestTenantHeader(t *testing.T) {
	a := newAPI(t, &fakeRunner{}, 100, 1<<20)
	tests := []struct {
		name, tenant string
		status       int
		code         string
	}{
		{"missing", "", 400, "missing_tenant"},
		{"unknown", "nobody", 404, "unknown_tenant"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e errEnvelope
			resp := a.do(t, http.MethodGet, "/runs/whatever", tt.tenant, "", &e)
			if resp.StatusCode != tt.status || e.Error.Code != tt.code {
				t.Errorf("got %d %q, want %d %q", resp.StatusCode, e.Error.Code, tt.status, tt.code)
			}
		})
	}
}

func TestCreateRun_Lifecycle(t *testing.T) {
	a := newAPI(t, &fakeRunner{}, 100, 1<<20)

	var rb runBody
	resp := a.do(t, http.MethodPost, "/runs", "acme", "http://a\n# comment\n\n  http://b  \n", &rb)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/runs/"+rb.ID {
		t.Errorf("Location = %q, want /runs/%s", loc, rb.ID)
	}
	if rb.Tenant != "acme" || rb.URLCount != 2 || rb.Status == "" {
		t.Errorf("accepted body = %+v", rb)
	}

	done := a.waitStatus(t, "acme", rb.ID, "done")
	if done.Summary == nil {
		t.Fatal("done run has no summary")
	}
	s := *done.Summary
	if s["total"] != float64(2) {
		t.Errorf("summary.total = %v, want 2", s["total"])
	}
	lat, _ := s["latency_ms"].(map[string]any)
	if lat["p50"] != float64(2) {
		t.Errorf("latency_ms.p50 = %v, want 2 (ms, not ns)", lat["p50"])
	}
	if codes, _ := s["status_codes"].(map[string]any); codes["200"] != float64(2) {
		t.Errorf("status_codes = %v, want {\"200\": 2}", s["status_codes"])
	}
}

func TestCreateRun_Caps(t *testing.T) {
	a := newAPI(t, &fakeRunner{}, 2, 64) // global cap 2 urls, body cap 64 bytes

	t.Run("empty body", func(t *testing.T) {
		var e errEnvelope
		resp := a.do(t, http.MethodPost, "/runs", "acme", "\n# nothing\n", &e)
		if resp.StatusCode != 400 || e.Error.Code != "no_urls" {
			t.Errorf("got %d %q, want 400 no_urls", resp.StatusCode, e.Error.Code)
		}
	})
	t.Run("too many urls for global cap", func(t *testing.T) {
		var e errEnvelope
		resp := a.do(t, http.MethodPost, "/runs", "acme", "a\nb\nc\n", &e)
		if resp.StatusCode != 413 || e.Error.Code != "too_many_urls" {
			t.Errorf("got %d %q, want 413 too_many_urls", resp.StatusCode, e.Error.Code)
		}
	})
	t.Run("tenant override allows more", func(t *testing.T) {
		resp := a.do(t, http.MethodPost, "/runs", "globex", "a\nb\nc\n", nil)
		if resp.StatusCode != 202 {
			t.Errorf("got %d, want 202 (globex max_urls=5)", resp.StatusCode)
		}
	})
	t.Run("body too large", func(t *testing.T) {
		var e errEnvelope
		resp := a.do(t, http.MethodPost, "/runs", "acme", strings.Repeat("x", 100), &e)
		if resp.StatusCode != 413 || e.Error.Code != "body_too_large" {
			t.Errorf("got %d %q, want 413 body_too_large", resp.StatusCode, e.Error.Code)
		}
	})
}

func TestCreateRun_ConflictWhileInProgress(t *testing.T) {
	block := make(chan struct{})
	a := newAPI(t, &fakeRunner{block: block}, 100, 1<<20)

	var first runBody
	if resp := a.do(t, http.MethodPost, "/runs", "acme", "http://a\n", &first); resp.StatusCode != 202 {
		t.Fatalf("first: %d", resp.StatusCode)
	}
	a.waitStatus(t, "acme", first.ID, "running")

	var e errEnvelope
	resp := a.do(t, http.MethodPost, "/runs", "acme", "http://a\n", &e)
	if resp.StatusCode != http.StatusConflict || e.Error.Code != "run_in_progress" {
		t.Errorf("second: got %d %q, want 409 run_in_progress", resp.StatusCode, e.Error.Code)
	}

	close(block)
	a.waitStatus(t, "acme", first.ID, "done")
}

func TestGetRun_CrossTenantIs404(t *testing.T) {
	a := newAPI(t, &fakeRunner{}, 100, 1<<20)
	var rb runBody
	a.do(t, http.MethodPost, "/runs", "acme", "http://a\n", &rb)

	var e errEnvelope
	resp := a.do(t, http.MethodGet, "/runs/"+rb.ID, "globex", "", &e)
	if resp.StatusCode != 404 || e.Error.Code != "run_not_found" {
		t.Errorf("got %d %q, want 404 run_not_found", resp.StatusCode, e.Error.Code)
	}
}

func TestUnknownRoute_UsesEnvelope(t *testing.T) {
	a := newAPI(t, &fakeRunner{}, 100, 1<<20)
	var e errEnvelope
	resp := a.do(t, http.MethodGet, "/nope", "", "", &e)
	if resp.StatusCode != 404 || e.Error.Code != "not_found" {
		t.Errorf("got %d %q, want 404 not_found", resp.StatusCode, e.Error.Code)
	}
}
