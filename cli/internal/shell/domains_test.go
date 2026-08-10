package shell

import (
	"testing"

	"github.com/intruder0007/Lumo/core/kernel"
)

func TestNewPlatformShell_RegistersAllEightDomains(t *testing.T) {
	s := NewPlatformShell(kernel.NewSessionStore())

	seen := map[string]bool{}
	start, _, ok := s.Current()
	if !ok {
		t.Fatal("NewPlatformShell should start with a focused panel")
	}
	domain := start
	for i := 0; i < 8; i++ {
		seen[string(domain)] = true
		s.Next()
		domain, _, _ = s.Current()
	}

	if len(seen) != 8 {
		t.Fatalf("visited %d distinct domains via Next, want 8: %v", len(seen), seen)
	}
	if domain != start {
		t.Fatalf("after 8 Next calls, focus = %v, want wrap back to %v", domain, start)
	}
}

func TestNewPlatformShell_FirstFocusedIsWorkspaceIntelligence(t *testing.T) {
	s := NewPlatformShell(kernel.NewSessionStore())
	domain, _, ok := s.Current()
	if !ok || domain != DomainWorkspaceIntelligence {
		t.Fatalf("Current = (%v, %v), want DomainWorkspaceIntelligence focused by default", domain, ok)
	}
}
