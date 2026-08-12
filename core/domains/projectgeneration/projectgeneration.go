// Package projectgeneration implements the Project Generation domain by
// wrapping the existing core/engine + core/registry — Lumo V1's full
// scaffolding implementation — behind the domain's model, per ADR-0017:
// "wrap, don't rewrite." It does not reimplement plugin resolution,
// capability ordering, or generation; Provider.Generate calls the same
// engine.Engine.Run used by the Stable `lumo new` command.
package projectgeneration

import (
	"github.com/intruder0007/Lumo/core/config"
	"github.com/intruder0007/Lumo/core/engine"
	"github.com/intruder0007/Lumo/core/registry"
)

// TemplateInfo is one discovered template plugin.
type TemplateInfo struct {
	Name, ProjectType, Language, Framework, Version string
}

// CapabilityInfo is one discovered capability plugin.
type CapabilityInfo struct {
	Name, CapabilityID, Version string
}

// GenerationRecord is the outcome of the most recent Generate call.
type GenerationRecord struct {
	TargetDir string
	Summary   engine.Summary
}

// Model is Project Generation's published fact-store model.
type Model struct {
	AvailableTemplates    []TemplateInfo
	AvailableCapabilities []CapabilityInfo
	LastGeneration        *GenerationRecord // nil if nothing generated this session
}

// Provider wraps a registry and engine for one Lumo run.
type Provider struct {
	reg  *registry.Registry
	eng  *engine.Engine
	last *GenerationRecord
}

// NewProvider returns a Provider using reg for plugin discovery/
// resolution and host to run resolved plugins — the same two
// collaborators engine.New already takes.
func NewProvider(reg *registry.Registry, host engine.Runner) *Provider {
	return &Provider{reg: reg, eng: engine.New(reg, host)}
}

// ListTemplates returns every discovered template plugin.
func (p *Provider) ListTemplates() ([]TemplateInfo, error) {
	plugins, err := p.reg.Discover()
	if err != nil {
		return nil, err
	}
	var out []TemplateInfo
	for _, pl := range plugins {
		if pl.Manifest.Kind == "template" {
			out = append(out, TemplateInfo{
				Name:        pl.Manifest.Name,
				ProjectType: pl.Manifest.ProjectType,
				Language:    pl.Manifest.Language,
				Framework:   pl.Manifest.Framework,
				Version:     pl.Manifest.Version,
			})
		}
	}
	return out, nil
}

// ListCapabilities returns every discovered capability plugin.
func (p *Provider) ListCapabilities() ([]CapabilityInfo, error) {
	plugins, err := p.reg.Discover()
	if err != nil {
		return nil, err
	}
	var out []CapabilityInfo
	for _, pl := range plugins {
		if pl.Manifest.Kind == "capability" {
			out = append(out, CapabilityInfo{
				Name:         pl.Manifest.Name,
				CapabilityID: pl.Manifest.CapabilityID,
				Version:      pl.Manifest.Version,
			})
		}
	}
	return out, nil
}

// Generate resolves the template and every requested capability,
// generates the project, and applies capabilities in dependency order —
// the same run engine.Engine.Run performs for the Stable `lumo new`
// command. On success it becomes the Provider's LastGeneration.
func (p *Provider) Generate(targetDir string, answers config.Answers) (engine.Summary, error) {
	summary, err := p.eng.Run(targetDir, answers)
	if err != nil {
		return summary, err
	}
	p.last = &GenerationRecord{TargetDir: targetDir, Summary: summary}
	return summary, nil
}

// Model returns the current template/capability listing plus the last
// successful generation, if any.
func (p *Provider) Model() (Model, error) {
	templates, err := p.ListTemplates()
	if err != nil {
		return Model{}, err
	}
	caps, err := p.ListCapabilities()
	if err != nil {
		return Model{}, err
	}
	return Model{AvailableTemplates: templates, AvailableCapabilities: caps, LastGeneration: p.last}, nil
}
