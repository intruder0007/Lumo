// Package connector runs outbound-only integrations (GitHub, SonarQube)
// as an ordered sequence of named phases (auth, fetch, validate, ...),
// short-circuiting on the first failure with a message naming which phase
// failed. See
// docs/superpowers/specs/2026-08-09-security-connectors-phase-status-design.md
// Section 3. This phase shape is scoped to connector operations only — it
// is not a general build/rollout-phase system for the rest of Lumo.
package connector

import (
	"fmt"

	"github.com/intruder0007/Lumo/core/diag"
)

// Result is what a connector reports after its phases finish (or after
// the phase engine short-circuits — see RunEngine).
type Result struct {
	Connected bool
	Detail    string
}

// Phase is one named step of a connector's operation (e.g. "auth",
// "fetch", "validate", "display"). Run performs the step; its returned
// Result is only meaningful for the last phase to run — RunEngine returns
// whatever the final executed phase returned.
type Phase struct {
	Name string
	Run  func() (Result, error)
}

// Connector is an outbound integration (GitHub, SonarQube) expressed as
// an ordered list of phases.
type Connector interface {
	Name() string
	Phases() []Phase
}

// PhaseError names which connector and phase failed, wrapping the
// underlying error so callers (e.g. `lumo status`) can print a specific
// message like "sonarqube: failed at auth: token rejected (401)" instead
// of a bare error.
type PhaseError struct {
	Connector, Phase string
	Err              error
}

func (e *PhaseError) Error() string {
	return fmt.Sprintf("%s: failed at %s: %v", e.Connector, e.Phase, e.Err)
}
func (e *PhaseError) Unwrap() error { return e.Err }

// RunEngine runs c's phases strictly in order, logging each phase
// transition through logger (see core/diag), and stops at the first
// phase that returns an error.
func RunEngine(c Connector, logger diag.Logger) (Result, error) {
	var last Result
	for _, p := range c.Phases() {
		logger.Logf("%s: phase %s starting", c.Name(), p.Name)
		res, err := p.Run()
		if err != nil {
			logger.Logf("%s: phase %s failed: %v", c.Name(), p.Name, err)
			return Result{}, &PhaseError{Connector: c.Name(), Phase: p.Name, Err: err}
		}
		logger.Logf("%s: phase %s done", c.Name(), p.Name)
		last = res
	}
	return last, nil
}
