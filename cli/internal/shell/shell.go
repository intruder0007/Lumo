// Package shell implements the Phase A3 TUI shell skeleton described in
// docs/architecture/domains/tui.md and the navigation direction in
// docs/architecture/tui-interaction-survey.md: a persistent "spine"
// listing every domain, with one domain's panel focused at a time.
//
// This is a skeleton: panels here are placeholders with no real domain
// data behind them (that starts in Phase B, per docs/architecture/v2-
// platform-architecture.md). Render produces a plain string, not
// terminal escape codes — this keeps the shell's navigation logic
// testable without a live TTY; the actual visual rendering (colors,
// layout, the accessibility guarantees carried over from the existing
// wizard) is a Phase A3 implementation detail layered on top of this
// skeleton, not fixed here.
package shell

import (
	"fmt"
	"strings"

	"github.com/intruder0007/Lumo/core/kernel"
)

// Panel is a placeholder for one domain's presentation. Real panels
// (Phase B) will read their domain's model from a kernel.FactStore;
// this skeleton's panels have no such dependency yet.
type Panel interface {
	// Title is shown in the spine.
	Title() string
	// Render returns the panel's content when it has focus.
	Render() string
}

// StubPanel is a Panel with fixed placeholder content, used for every
// domain until its Phase B implementation lands.
type StubPanel struct {
	title   string
	content string
}

// NewStubPanel returns a StubPanel with the given spine title and
// placeholder content.
func NewStubPanel(title, content string) StubPanel {
	return StubPanel{title: title, content: content}
}

func (p StubPanel) Title() string  { return p.title }
func (p StubPanel) Render() string { return p.content }

// sessionFocusKey is the SessionStore key the shell uses to remember
// which panel has focus, per docs/architecture/domains/kernel.md §3
// (Session Store: ephemeral, in-memory, TUI-only state).
const sessionFocusKey = "focused-panel"

// entry pairs a registered domain with its panel, keeping registration
// order — the spine lists domains in the order they were registered.
type entry struct {
	id    kernel.DomainID
	panel Panel
}

// Shell is the persistent navigation shell: a spine of registered
// domain panels plus one focused panel at a time.
type Shell struct {
	session *kernel.SessionStore
	entries []entry
}

// New returns an empty Shell. session is where the shell persists which
// panel is focused, so the answer survives across re-renders within one
// run (it is not written to disk — SessionStore never is).
func New(session *kernel.SessionStore) *Shell {
	return &Shell{session: session}
}

// UnknownPanelError is returned by Focus when no panel is registered
// under the given domain.
type UnknownPanelError struct{ Domain kernel.DomainID }

func (e *UnknownPanelError) Error() string {
	return fmt.Sprintf("shell: no panel registered for domain %q", e.Domain)
}

// RegisterPanel adds panel to the spine under domain. The first
// registered panel becomes focused by default. Registering the same
// domain twice replaces its panel but keeps its original spine
// position.
func (s *Shell) RegisterPanel(domain kernel.DomainID, panel Panel) {
	for i, e := range s.entries {
		if e.id == domain {
			s.entries[i].panel = panel
			return
		}
	}
	s.entries = append(s.entries, entry{id: domain, panel: panel})
	if len(s.entries) == 1 {
		s.session.Set(sessionFocusKey, domain)
	}
}

// Focus switches the focused panel to domain. It returns
// *UnknownPanelError if no panel is registered under domain.
func (s *Shell) Focus(domain kernel.DomainID) error {
	for _, e := range s.entries {
		if e.id == domain {
			s.session.Set(sessionFocusKey, domain)
			return nil
		}
	}
	return &UnknownPanelError{Domain: domain}
}

// Current returns the currently focused domain and panel. ok is false
// if no panel has been registered yet.
func (s *Shell) Current() (domain kernel.DomainID, panel Panel, ok bool) {
	raw, has := s.session.Get(sessionFocusKey)
	if !has {
		return "", nil, false
	}
	focused := raw.(kernel.DomainID)
	for _, e := range s.entries {
		if e.id == focused {
			return e.id, e.panel, true
		}
	}
	return "", nil, false
}

// Next focuses the panel after the current one in registration order,
// wrapping around after the last one. It is a no-op if no panel is
// registered.
func (s *Shell) Next() {
	s.step(1)
}

// Prev focuses the panel before the current one in registration order,
// wrapping around before the first one. It is a no-op if no panel is
// registered.
func (s *Shell) Prev() {
	s.step(-1)
}

func (s *Shell) step(delta int) {
	if len(s.entries) == 0 {
		return
	}
	current, _, ok := s.Current()
	if !ok {
		s.session.Set(sessionFocusKey, s.entries[0].id)
		return
	}
	idx := 0
	for i, e := range s.entries {
		if e.id == current {
			idx = i
			break
		}
	}
	next := (idx + delta + len(s.entries)) % len(s.entries)
	s.session.Set(sessionFocusKey, s.entries[next].id)
}

// Render returns the spine (every registered domain's title, with the
// focused one marked) followed by the focused panel's content. Per the
// interaction survey's P1 (persistent spine, not a tab bar), the spine
// is always shown in full, never collapsed to the current panel alone.
func (s *Shell) Render() string {
	var b strings.Builder
	focused, panel, ok := s.Current()

	for i, e := range s.entries {
		if i > 0 {
			b.WriteString("  ")
		}
		if ok && e.id == focused {
			fmt.Fprintf(&b, "[%s]", e.panel.Title())
		} else {
			b.WriteString(e.panel.Title())
		}
	}
	b.WriteString("\n\n")

	if ok {
		b.WriteString(panel.Render())
	}
	return b.String()
}
