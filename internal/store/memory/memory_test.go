package memory

import (
	"context"
	"errors"
	"testing"

	"github.com/hemantksingh/go-starter/internal/healthrun"
)

func TestRunStore_CRUD(t *testing.T) {
	ctx := context.Background()
	s := NewRunStore()

	run := healthrun.Run{ID: "r1", TenantID: "acme", Status: healthrun.StatusQueued}
	if err := s.Create(ctx, run); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get(ctx, "acme", "r1")
	if err != nil || got.Status != healthrun.StatusQueued {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	// Mutating the returned value must not touch the stored one.
	got.Status = healthrun.StatusFailed
	again, _ := s.Get(ctx, "acme", "r1")
	if again.Status != healthrun.StatusQueued {
		t.Errorf("Get returned shared memory: stored status became %s", again.Status)
	}

	run.Status = healthrun.StatusDone
	if err := s.Update(ctx, run); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, _ := s.Get(ctx, "acme", "r1")
	if updated.Status != healthrun.StatusDone {
		t.Errorf("after Update status = %s, want done", updated.Status)
	}
}

func TestRunStore_TenantScoped(t *testing.T) {
	ctx := context.Background()
	s := NewRunStore()
	_ = s.Create(ctx, healthrun.Run{ID: "r1", TenantID: "acme"})

	if _, err := s.Get(ctx, "globex", "r1"); !errors.Is(err, healthrun.ErrNotFound) {
		t.Errorf("cross-tenant Get err = %v, want ErrNotFound", err)
	}
}

func TestRunStore_UpdateMissing(t *testing.T) {
	s := NewRunStore()
	err := s.Update(context.Background(), healthrun.Run{ID: "ghost", TenantID: "acme"})
	if !errors.Is(err, healthrun.ErrNotFound) {
		t.Errorf("Update(missing) err = %v, want ErrNotFound", err)
	}
}
