package kernel

import (
	"path/filepath"
	"testing"
)

func TestConfigStore_GetUnsetKey(t *testing.T) {
	c, err := NewConfigStore(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}
	if _, ok := c.Get("theme"); ok {
		t.Fatal("Get on an unset key should return ok=false")
	}
}

func TestConfigStore_SetThenGet(t *testing.T) {
	c, err := NewConfigStore(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}
	if err := c.Set("theme", "minimal"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok := c.Get("theme")
	if !ok || got != "minimal" {
		t.Fatalf("Get after Set = (%q, %v), want (\"minimal\", true)", got, ok)
	}
}

func TestConfigStore_PersistsAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	first, err := NewConfigStore(path)
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}
	if err := first.Set("theme", "minimal"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	second, err := NewConfigStore(path)
	if err != nil {
		t.Fatalf("NewConfigStore (reload): %v", err)
	}
	got, ok := second.Get("theme")
	if !ok || got != "minimal" {
		t.Fatalf("Get on reloaded store = (%q, %v), want (\"minimal\", true)", got, ok)
	}
}

func TestConfigStore_MissingFileIsNotAnError(t *testing.T) {
	if _, err := NewConfigStore(filepath.Join(t.TempDir(), "does-not-exist.json")); err != nil {
		t.Fatalf("NewConfigStore on a missing file: %v, want nil error", err)
	}
}

func TestSessionStore_GetUnsetKey(t *testing.T) {
	s := NewSessionStore()
	if _, ok := s.Get("focused-panel"); ok {
		t.Fatal("Get on an unset key should return ok=false")
	}
}

func TestSessionStore_SetThenGet(t *testing.T) {
	s := NewSessionStore()
	s.Set("focused-panel", "source-control")
	got, ok := s.Get("focused-panel")
	if !ok || got != "source-control" {
		t.Fatalf("Get after Set = (%v, %v), want (\"source-control\", true)", got, ok)
	}
}

func TestSessionStore_DoesNotPersistToDisk(t *testing.T) {
	// SessionStore has no path/file argument at all — this test documents
	// that as an API-shape guarantee, not just an implementation detail.
	s := NewSessionStore()
	s.Set("k", "v")
	s2 := NewSessionStore()
	if _, ok := s2.Get("k"); ok {
		t.Fatal("a new SessionStore should not see another instance's data")
	}
}
