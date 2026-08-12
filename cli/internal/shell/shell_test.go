package shell

import (
	"errors"
	"strings"
	"testing"

	"github.com/intruder0007/Lumo/core/kernel"
)

func newTestShell() *Shell {
	return New(kernel.NewSessionStore())
}

func TestShell_CurrentEmptyShell(t *testing.T) {
	s := newTestShell()
	if _, _, ok := s.Current(); ok {
		t.Fatal("Current on an empty shell should return ok=false")
	}
}

func TestShell_FirstRegisteredPanelIsFocusedByDefault(t *testing.T) {
	s := newTestShell()
	s.RegisterPanel("workspace-intelligence", NewStubPanel("Workspace", "ws content"))
	s.RegisterPanel("source-control", NewStubPanel("Source Control", "scm content"))

	domain, panel, ok := s.Current()
	if !ok || domain != "workspace-intelligence" || panel.Render() != "ws content" {
		t.Fatalf("Current = (%v, %v, %v), want first-registered panel focused", domain, panel, ok)
	}
}

func TestShell_FocusUnknownDomain(t *testing.T) {
	s := newTestShell()
	s.RegisterPanel("workspace-intelligence", NewStubPanel("Workspace", "ws content"))

	err := s.Focus("does-not-exist")
	var want *UnknownPanelError
	if !errors.As(err, &want) {
		t.Fatalf("Focus(unknown) = %v, want *UnknownPanelError", err)
	}
}

func TestShell_FocusSwitchesCurrent(t *testing.T) {
	s := newTestShell()
	s.RegisterPanel("workspace-intelligence", NewStubPanel("Workspace", "ws content"))
	s.RegisterPanel("source-control", NewStubPanel("Source Control", "scm content"))

	if err := s.Focus("source-control"); err != nil {
		t.Fatalf("Focus: %v", err)
	}
	domain, _, ok := s.Current()
	if !ok || domain != "source-control" {
		t.Fatalf("Current after Focus = (%v, %v), want source-control", domain, ok)
	}
}

func TestShell_NextWrapsAround(t *testing.T) {
	s := newTestShell()
	s.RegisterPanel("a", NewStubPanel("A", ""))
	s.RegisterPanel("b", NewStubPanel("B", ""))

	s.Next() // a -> b
	if d, _, _ := s.Current(); d != "b" {
		t.Fatalf("after one Next, focus = %v, want b", d)
	}
	s.Next() // b -> wraps to a
	if d, _, _ := s.Current(); d != "a" {
		t.Fatalf("after two Next calls, focus = %v, want a (wrapped)", d)
	}
}

func TestShell_PrevWrapsAround(t *testing.T) {
	s := newTestShell()
	s.RegisterPanel("a", NewStubPanel("A", ""))
	s.RegisterPanel("b", NewStubPanel("B", ""))

	s.Prev() // a -> wraps to b
	if d, _, _ := s.Current(); d != "b" {
		t.Fatalf("after one Prev, focus = %v, want b (wrapped)", d)
	}
}

func TestShell_NextOnEmptyShellIsNoop(t *testing.T) {
	s := newTestShell()
	s.Next()
	if _, _, ok := s.Current(); ok {
		t.Fatal("Next on an empty shell should not fabricate a focused panel")
	}
}

func TestShell_RegisteringSameDomainTwiceReplacesInPlace(t *testing.T) {
	s := newTestShell()
	s.RegisterPanel("a", NewStubPanel("A", "first"))
	s.RegisterPanel("b", NewStubPanel("B", ""))
	s.RegisterPanel("a", NewStubPanel("A", "second"))

	if err := s.Focus("a"); err != nil {
		t.Fatalf("Focus: %v", err)
	}
	_, panel, _ := s.Current()
	if panel.Render() != "second" {
		t.Fatalf("re-registered panel content = %q, want %q", panel.Render(), "second")
	}

	// Position preserved: Next from "a" should still go to "b", not skip it.
	s.Next()
	if d, _, _ := s.Current(); d != "b" {
		t.Fatalf("focus after Next = %v, want b (registration order preserved)", d)
	}
}

func TestShell_RenderShowsFullSpineAndFocusedContent(t *testing.T) {
	s := newTestShell()
	s.RegisterPanel("workspace-intelligence", NewStubPanel("Workspace", "ws content"))
	s.RegisterPanel("source-control", NewStubPanel("Source Control", "scm content"))

	out := s.Render()
	if !strings.Contains(out, "[Workspace]") {
		t.Errorf("Render output missing focused marker for the current panel:\n%s", out)
	}
	if !strings.Contains(out, "Source Control") {
		t.Errorf("Render output missing the unfocused panel's title (spine must show all panels):\n%s", out)
	}
	if !strings.Contains(out, "ws content") {
		t.Errorf("Render output missing the focused panel's content:\n%s", out)
	}
	if strings.Contains(out, "scm content") {
		t.Errorf("Render output should not include the unfocused panel's content:\n%s", out)
	}
}
