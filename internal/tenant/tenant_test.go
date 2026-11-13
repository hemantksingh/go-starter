package tenant

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNew_ValidatesIDs(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		valid bool
	}{
		{"simple", "acme", true},
		{"with digits and dash", "acme-2", true},
		{"uppercase", "Acme", false},
		{"leading dash", "-acme", false},
		{"space", "ac me", false},
		{"empty", "", false},
		{"underscore", "ac_me", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New([]Tenant{{ID: tt.id, Name: "x"}})
			if (err == nil) != tt.valid {
				t.Errorf("New(id=%q) err = %v, want valid=%v", tt.id, err, tt.valid)
			}
		})
	}
}

func TestNew_RejectsDuplicatesEmptyAndNegativeCap(t *testing.T) {
	if _, err := New([]Tenant{{ID: "a"}, {ID: "a"}}); err == nil {
		t.Error("duplicate id accepted")
	}
	if _, err := New(nil); err == nil {
		t.Error("empty list accepted")
	}
	if _, err := New([]Tenant{{ID: "a", MaxURLs: -1}}); err == nil {
		t.Error("negative max_urls accepted")
	}
}

func TestLoadFile_AndLookup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenants.json")
	data := `[{"id":"acme","name":"Acme"},{"id":"globex","name":"Globex","max_urls":100}]`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	reg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if reg.Len() != 2 {
		t.Errorf("Len = %d, want 2", reg.Len())
	}
	g, ok := reg.Lookup("globex")
	if !ok || g.Name != "Globex" || g.MaxURLs != 100 {
		t.Errorf("Lookup(globex) = %+v, %v", g, ok)
	}
	if _, ok := reg.Lookup("nope"); ok {
		t.Error("Lookup(nope) found something")
	}
}

func TestLoadFile_Errors(t *testing.T) {
	if _, err := LoadFile(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("missing file accepted")
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(bad, []byte("{not json"), 0o600)
	if _, err := LoadFile(bad); err == nil {
		t.Error("malformed json accepted")
	}
}

func TestContext_RoundTrip(t *testing.T) {
	if got := FromContext(context.Background()); got != nil {
		t.Errorf("FromContext(empty) = %v, want nil", got)
	}
	want := &Tenant{ID: "acme"}
	ctx := WithContext(context.Background(), want)
	if got := FromContext(ctx); got != want {
		t.Errorf("FromContext = %p, want %p", got, want)
	}
}
