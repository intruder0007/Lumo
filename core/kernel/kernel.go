// Package kernel implements the platform kernel described in
// docs/architecture/domains/kernel.md (ADR-0017): the shared seam that
// lets independently-owned domains (Workspace Intelligence, Project
// Generation, Source Control, Security Center, Quality, Dev Environment,
// Automation, TUI) see each other's state without depending on each
// other's internals.
//
// The kernel knows nothing about what a domain's data means — it only
// moves opaque models (FactStore), notifications (EventBus), and
// key/value settings (ConfigStore, SessionStore) between domains that do.
// This package has no dependency on any domain package, by design: the
// Phase A2 exit criterion is that the kernel compiles and is tested with
// zero domain code depending on it yet.
package kernel

// DomainID identifies a domain for the kernel's purposes. The kernel
// treats it as an opaque string; the set of domain IDs in use is a
// decision each domain and its consumers make, not something this
// package defines or validates.
type DomainID string
