package httpapi

// In-package tests: they reach the unexported requestInfo plumbing.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hemantksingh/go-starter/internal/tenant"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestObservability_RequestID(t *testing.T) {
	var seen string
	h := Observability(zap.NewNop())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFromContext(r.Context())
	}))

	t.Run("generated when absent", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		got := rec.Header().Get("X-Request-ID")
		if len(got) != 32 || got != seen {
			t.Errorf("header %q, ctx %q: want 32-char id, equal", got, seen)
		}
	})
	t.Run("echoed when present", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Request-ID", "client-chosen")
		h.ServeHTTP(rec, req)
		if rec.Header().Get("X-Request-ID") != "client-chosen" || seen != "client-chosen" {
			t.Errorf("header %q, ctx %q: want client-chosen", rec.Header().Get("X-Request-ID"), seen)
		}
	})
}

func TestObservability_RecoversPanic(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	h := Observability(zap.New(core))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("kaboom")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil)) // must not propagate

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	var e struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&e); err != nil || e.Error.Code != "internal_error" {
		t.Errorf("body = %s (err %v), want internal_error envelope", rec.Body.String(), err)
	}
	if logs.FilterMessage("panic recovered").Len() != 1 {
		t.Error("panic was not logged")
	}
}

func TestObservability_LogsStatusAndTenant(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	reg, _ := tenant.New([]tenant.Tenant{{ID: "acme"}})

	// Observability outside, Tenant inside: the tenant middleware fills the
	// requestInfo the outer one logs after the handler returns.
	inner := Tenant(reg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	h := Observability(zap.New(core))(inner)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Tenant-ID", "acme")
	h.ServeHTTP(httptest.NewRecorder(), req)

	entries := logs.FilterMessage("request").All()
	if len(entries) != 1 {
		t.Fatalf("got %d access-log lines, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["status"] != int64(http.StatusTeapot) {
		t.Errorf("status field = %v, want 418", fields["status"])
	}
	if fields["tenant"] != "acme" {
		t.Errorf("tenant field = %v, want acme (inner middleware should fill outer's info)", fields["tenant"])
	}
}

func TestTenant_PutsTenantOnContext(t *testing.T) {
	reg, _ := tenant.New([]tenant.Tenant{{ID: "acme", Name: "Acme"}})
	var got *tenant.Tenant
	h := Tenant(reg)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = tenant.FromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Tenant-ID", "acme")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil || got.Name != "Acme" {
		t.Errorf("tenant on context = %+v, want Acme", got)
	}
}
