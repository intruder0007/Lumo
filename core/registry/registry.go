// Package registry discovers plugins (templates and capabilities) by
// scanning local directories for plugin.json manifests, and resolves
// wizard answers to the plugin that should handle them. See
// docs/architecture/plugin-protocol.md "Discovery".
package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	sdk "github.com/intruder0007/Lumo/sdk/go/sdk"
)

// Plugin pairs a discovered manifest with the absolute path to its
// executable entrypoint.
type Plugin struct {
	Manifest       sdk.Manifest
	EntrypointPath string
}

// DiscoveryIssue records a plugin.json that was found but skipped,
// because it failed to parse or failed Manifest.Validate(). A single
// broken third-party plugin should never prevent discovery of everything
// else — see DiscoverWithIssues.
type DiscoveryIssue struct {
	Path string
	Err  error
}

func (i DiscoveryIssue) Error() string {
	return fmt.Sprintf("%s: %v", i.Path, i.Err)
}

// TemplateNotFoundError is returned by ResolveTemplate when no
// discovered template plugin matches.
type TemplateNotFoundError struct {
	ProjectType, Language, Framework string
}

func (e *TemplateNotFoundError) Error() string {
	return fmt.Sprintf("no template plugin found for project type %q, language %q, framework %q", e.ProjectType, e.Language, e.Framework)
}

// CapabilityNotFoundError is returned by ResolveCapability when no
// discovered capability plugin has the requested capabilityId.
type CapabilityNotFoundError struct {
	CapabilityID string
}

func (e *CapabilityNotFoundError) Error() string {
	return fmt.Sprintf("no capability plugin found for id %q", e.CapabilityID)
}

// Registry discovers plugins under a fixed set of local directories.
// The scan result is cached after the first call (see discover) — a
// Registry represents one point-in-time view of dirs, matching how
// every call site uses it: constructed fresh right before a single
// command's plugin resolution, never expected to observe filesystem
// changes mid-command.
type Registry struct {
	dirs []string

	scanOnce    sync.Once
	scanPlugins []Plugin
	scanIssues  []DiscoveryIssue
	scanErr     error
}

// New returns a Registry that scans the given directories (each
// directory's immediate subdirectories are checked for a plugin.json).
func New(dirs ...string) *Registry {
	return &Registry{dirs: dirs}
}

// Discover scans all configured directories and returns every *valid*
// plugin found. Manifests that fail to parse or fail Validate() are
// skipped, not errored on — use DiscoverWithIssues to see what was
// skipped and why (e.g. for `lumo plugins list` diagnostics).
func (r *Registry) Discover() ([]Plugin, error) {
	plugins, _, err := r.discover()
	return plugins, err
}

// DiscoverWithIssues is like Discover, but also returns manifests that
// were found but skipped.
func (r *Registry) DiscoverWithIssues() ([]Plugin, []DiscoveryIssue, error) {
	return r.discover()
}

// discover scans r.dirs exactly once per Registry instance and caches
// the result (sync.Once, so it's also safe if ever called concurrently)
// — ResolveTemplate and ResolveCapability each call this, and a single
// `lumo new` run with N selected capabilities previously re-scanned
// every configured plugin directory and re-parsed every plugin.json
// N+1 times over (once for the template, once per capability) for
// identical results each time.
func (r *Registry) discover() ([]Plugin, []DiscoveryIssue, error) {
	r.scanOnce.Do(func() {
		var found []Plugin
		var issues []DiscoveryIssue
		for _, dir := range r.dirs {
			dirFound, dirIssues, err := scanDir(dir)
			if err != nil {
				r.scanErr = err
				return
			}
			found = append(found, dirFound...)
			issues = append(issues, dirIssues...)
		}
		r.scanPlugins = found
		r.scanIssues = issues
	})
	return r.scanPlugins, r.scanIssues, r.scanErr
}

// scanDir checks each immediate subdirectory of dir for a plugin.json. A
// missing dir is not an error (V1 ships with only some of templates/,
// plugins/ present depending on what's merged).
func scanDir(dir string) ([]Plugin, []DiscoveryIssue, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("registry: reading %s: %w", dir, err)
	}

	var found []Plugin
	var issues []DiscoveryIssue
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pluginDir := filepath.Join(dir, entry.Name())
		plugin, issue, ok := loadPluginDir(pluginDir)
		switch {
		case issue != nil:
			issues = append(issues, *issue)
		case ok:
			found = append(found, plugin)
		}
	}
	return found, issues, nil
}

// loadPluginDir reads and validates pluginDir/plugin.json. ok is false
// with no issue when pluginDir isn't a plugin directory at all (no
// plugin.json) — that's not an error, just not a match.
func loadPluginDir(pluginDir string) (plugin Plugin, issue *DiscoveryIssue, ok bool) {
	manifestPath := filepath.Join(pluginDir, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return Plugin{}, nil, false
	}

	var m sdk.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Plugin{}, &DiscoveryIssue{Path: manifestPath, Err: fmt.Errorf("parsing manifest: %w", err)}, false
	}
	if err := m.Validate(); err != nil {
		return Plugin{}, &DiscoveryIssue{Path: manifestPath, Err: err}, false
	}

	return Plugin{
		Manifest:       m,
		EntrypointPath: resolveEntrypoint(pluginDir, m.Entrypoint),
	}, nil, true
}

// LoadPluginDir parses and validates a single plugin directory (not a
// scan): pluginDir/plugin.json must parse and pass Validate(), and the
// entrypoint is resolved the same way discovery would. ok is false when
// pluginDir isn't a plugin directory at all (no plugin.json) — same
// semantics as discovery's skip. This is the single-directory version
// of the discovery path, used by `lumo plugins validate <dir>`.
func LoadPluginDir(pluginDir string) (plugin Plugin, ok bool, err error) {
	p, issue, ok := loadPluginDir(pluginDir)
	if issue != nil {
		return Plugin{}, false, issue
	}
	return p, ok, nil
}

// resolveEntrypoint joins pluginDir and entrypoint, and on platforms
// (Windows) where exec.Command requires an explicit executable
// extension, falls back to entrypoint+".exe" if the extensionless path
// doesn't exist. Go's exec.Command does not auto-resolve PATHEXT for
// explicit relative/absolute paths, only for bare names via LookPath.
func resolveEntrypoint(pluginDir, entrypoint string) string {
	base := filepath.Join(pluginDir, entrypoint)
	if _, err := os.Stat(base); err == nil {
		return base
	}
	if _, err := os.Stat(base + ".exe"); err == nil {
		return base + ".exe"
	}
	return base
}

// ResolveTemplate finds a template plugin matching the given project
// type, language, and framework, that also supports the current
// platform (see Manifest.SupportedPlatforms).
func (r *Registry) ResolveTemplate(projectType, language, framework string) (Plugin, error) {
	plugins, err := r.Discover()
	if err != nil {
		return Plugin{}, err
	}
	for _, p := range plugins {
		if p.Manifest.Kind != "template" {
			continue
		}
		if p.Manifest.ProjectType != projectType || p.Manifest.Language != language || p.Manifest.Framework != framework {
			continue
		}
		if !supportsCurrentPlatform(p.Manifest) {
			continue
		}
		return p, nil
	}
	return Plugin{}, &TemplateNotFoundError{ProjectType: projectType, Language: language, Framework: framework}
}

// supportsCurrentPlatform reports whether m.SupportedPlatforms includes
// runtime.GOOS. An empty list means "all platforms."
func supportsCurrentPlatform(m sdk.Manifest) bool {
	return supportsPlatform(m, runtime.GOOS)
}

func supportsPlatform(m sdk.Manifest, goos string) bool {
	if len(m.SupportedPlatforms) == 0 {
		return true
	}
	for _, p := range m.SupportedPlatforms {
		if p == goos {
			return true
		}
	}
	return false
}

// ResolveCapability finds a capability plugin by its capabilityId.
func (r *Registry) ResolveCapability(capabilityID string) (Plugin, error) {
	plugins, err := r.Discover()
	if err != nil {
		return Plugin{}, err
	}
	for _, p := range plugins {
		if p.Manifest.Kind == "capability" && p.Manifest.CapabilityID == capabilityID {
			return p, nil
		}
	}
	return Plugin{}, &CapabilityNotFoundError{CapabilityID: capabilityID}
}
