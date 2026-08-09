// Command lumo is the Lumo interactive/non-interactive wizard. See
// docs/cli/usage.md and ADR-0007.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/intruder0007/Lumo/cli/internal/embedded"
	"github.com/intruder0007/Lumo/cli/internal/prompt"
	"github.com/intruder0007/Lumo/core/config"
	"github.com/intruder0007/Lumo/core/connector"
	"github.com/intruder0007/Lumo/core/diag"
	"github.com/intruder0007/Lumo/core/engine"
	"github.com/intruder0007/Lumo/core/plugin"
	"github.com/intruder0007/Lumo/core/registry"
	"github.com/intruder0007/Lumo/core/secretstore"
	sdk "github.com/intruder0007/Lumo/sdk/go/sdk"
)

// version is overridden at build time via:
//
//	go build -ldflags "-X main.version=vX.Y.Z" ./cli
//
// See ADR-0006 and .github/workflows/release.yml. Left as "dev" for
// ordinary local builds (go run, go build with no ldflags, go install).
var version = "dev"

func main() {
	// Set the window title to Lumo for the whole session (design-system
	// §6); interactive flows refine it as they progress, and exit restores
	// the caller's title. TTY-gated inside SetTitle, so piped output is
	// untouched and goldens stay stable.
	prompt.SetTitle("Lumo")
	defer prompt.RestoreTitle() // runs on normal return; exit() covers os.Exit paths

	if len(os.Args) < 2 {
		// No command given (e.g. double-clicking lumo.exe, or typing
		// `lumo` at a shell): start the interactive wizard — the
		// intuitive behavior for personal use. Under piped/non-terminal
		// stdin the wizard's line fallback hits EOF and fails with
		// "project name is required" (exit 1); scripts should always pass
		// a command. `lumo help` still prints the command reference.
		cmdNew(nil)
		return
	}

	switch os.Args[1] {
	case "new":
		cmdNew(os.Args[2:])
	case "plugins":
		cmdPlugins(os.Args[2:])
	case "config":
		cmdConfig(os.Args[2:])
	case "doctor":
		cmdDoctor(os.Args[2:])
	case "status":
		cmdStatus(os.Args[2:])
	case "version":
		fmt.Printf("lumo version %s (%s, %s/%s)\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	case "-h", "--help", "help":
		fmt.Println(prompt.HelpText)
	default:
		fmt.Fprintln(os.Stderr, prompt.HelpText)
		exit(1)
	}
}

// exit terminates with the given code after restoring the terminal title
// the session set (os.Exit skips deferred functions, so the restore must
// be explicit). Calls the platform's title restore, then os.Exit.
func exit(code int) {
	prompt.RestoreTitle()
	os.Exit(code)
}

// pluginDirs returns the local directories the registry scans: an
// explicit LUMO_PLUGIN_DIRS override (os.PathListSeparator-delimited, for
// installed binaries or tests where neither the executable's directory
// nor the working directory is the repo root) takes priority, then
// directories relative to the running executable, then the current
// working directory (matching a repo-root `./bin/lumo new` dev
// workflow). Deduplicated by absolute path, since the executable's
// directory and the working directory are the same thing for the most
// common real usage — cd into an extracted release archive and run
// ./lumo — which would otherwise register every plugin twice.
//
// If none of those candidates actually contain a plugin (e.g. a bare
// `go install`-produced binary, with no sibling directories at all), the
// V1 plugin set embedded into this binary at build time (see
// cli/internal/embedded) is self-extracted to a version-scoped cache
// directory and appended as a last-resort fallback — see ADR-0012. An
// explicit LUMO_PLUGIN_DIRS override always wins and skips this fallback
// entirely, on the theory that an explicit override reflects deliberate
// intent, even if it happens to point somewhere empty.
func pluginDirs() []string {
	if override := os.Getenv("LUMO_PLUGIN_DIRS"); override != "" {
		return dedupeAbs(strings.Split(override, string(os.PathListSeparator)))
	}

	dirs := []string{filepath.Join("templates"), filepath.Join("plugins", "builtin")}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		dirs = append([]string{
			filepath.Join(exeDir, "templates"),
			filepath.Join(exeDir, "plugins", "builtin"),
		}, dirs...)
	}
	dirs = dedupeAbs(dirs)

	if hasAnyPlugin(dirs) {
		return dirs
	}
	if cacheDir, err := embeddedFallbackDir(); err == nil {
		// registry.scanDir treats each configured directory's immediate
		// children as individual plugin directories, so — matching the
		// "templates" / "plugins/builtin" pair above, not the bare
		// parent — the fallback must append the two subdirectories
		// ExtractTo populated, not the cache root itself.
		return append(dirs,
			filepath.Join(cacheDir, "templates"),
			filepath.Join(cacheDir, "plugins", "builtin"),
		)
	}
	return dirs
}

// hasAnyPlugin reports whether any immediate subdirectory of any dir in
// dirs contains a plugin.json — a cheap existence probe, not a full
// registry.Discover() (no parsing/validation), used only to decide
// whether the embedded fallback should engage.
func hasAnyPlugin(dirs []string) bool {
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, e.Name(), "plugin.json")); err == nil {
				return true
			}
		}
	}
	return false
}

// embeddedCacheDir returns the version-scoped directory the embedded
// plugin fallback extracts to (os.UserCacheDir()/lumo/<version>/,
// mirroring cli/internal/prompt/config.go's use of os.UserConfigDir()
// for the same kind of per-user, per-OS standard directory). Scoping by
// version means an upgrade can never serve stale plugins from a
// previous version's cache.
func embeddedCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "lumo", version), nil
}

// embeddedFallbackDir extracts (if needed) and returns the embedded
// fallback's cache directory, or an error if this binary has no
// embedded assets to fall back to (e.g. a plain `go build ./cli` run
// without the Makefile's staging step).
func embeddedFallbackDir() (string, error) {
	if !embedded.Available() {
		return "", fmt.Errorf("no plugin assets embedded in this binary")
	}
	dir, err := embeddedCacheDir()
	if err != nil {
		return "", err
	}
	if err := embedded.ExtractTo(dir); err != nil {
		return "", err
	}
	return dir, nil
}

func dedupeAbs(dirs []string) []string {
	seen := make(map[string]bool, len(dirs))
	var out []string
	for _, d := range dirs {
		abs, err := filepath.Abs(d)
		if err != nil {
			abs = d
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, d)
	}
	return out
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, tok := range strings.Split(s, ",") {
		tok = strings.TrimSpace(tok)
		if tok != "" {
			out = append(out, tok)
		}
	}
	return out
}

// wizardSpecFromDiscovery reduces discovered plugins to the plain data
// the wizard renders from (prompt.WizardSpec) — the registry-driven
// replacement for the old hardcoded option lists, so the wizard always
// offers exactly what's installed (see ADR-0007 and
// docs/architecture/roadmap.md).
func wizardSpecFromDiscovery(plugins []registry.Plugin) prompt.WizardSpec {
	var spec prompt.WizardSpec
	for _, p := range plugins {
		switch p.Manifest.Kind {
		case "template":
			spec.Templates = append(spec.Templates, prompt.TemplateSpec{
				ProjectType: p.Manifest.ProjectType,
				Language:    p.Manifest.Language,
				Framework:   p.Manifest.Framework,
				DisplayName: p.Manifest.DisplayName,
			})
		case "capability":
			spec.Capabilities = append(spec.Capabilities, prompt.CapabilitySpec{
				ID:          p.Manifest.CapabilityID,
				DisplayName: p.Manifest.DisplayName,
			})
		}
	}
	return spec
}

// resolveTargetPath splits a `lumo new` positional into the absolute
// target directory and the project name — the shared rule is
// prompt.ResolveTargetPath, used by the wizard's confirm screen too.
func resolveTargetPath(positional string) (targetDir, projectName string, err error) {
	return prompt.ResolveTargetPath(positional)
}

// extractProjectName pulls the positional project-name argument out of
// args regardless of where it appears relative to flags (docs/cli/usage.md
// documents "lumo new my-project --theme ..."), since the stdlib
// flag package stops parsing at the first non-flag token and would
// otherwise silently swallow every flag after a leading positional arg.
func extractProjectName(args []string, boolFlags map[string]bool) (string, []string) {
	var rest []string
	projectName := ""
	skipNext := false
	for _, a := range args {
		if skipNext {
			rest = append(rest, a)
			skipNext = false
			continue
		}
		if strings.HasPrefix(a, "-") {
			rest = append(rest, a)
			if !boolFlags[a] && !strings.Contains(a, "=") {
				skipNext = true
			}
			continue
		}
		if projectName == "" {
			projectName = a
			continue
		}
		rest = append(rest, a)
	}
	return projectName, rest
}

func cmdNew(args []string) {
	positional, rest := extractProjectName(args, map[string]bool{
		"-no-color": true, "--no-color": true,
		"-verbose": true, "--verbose": true,
		"-v": true, "--v": true,
		"-yes": true, "--yes": true,
	})

	fs := flag.NewFlagSet("new", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: lumo new [project-name-or-path] [flags]

Generates a new project. With no flags and no --answers, runs the
interactive wizard (which always asks where to generate — see -dir
below for the non-interactive equivalent). Flags below make it
non-interactive.

The positional argument may be a bare project name (generated in the
current directory, unless -dir is set) or a target path — relative
(./my-app, ../x/app), absolute (/home/me/app, C:\code\app), or
~-prefixed (~/code/app). The project name is derived from the path's
final component. -dir and a path-like project name can't be combined.`)
		fs.PrintDefaults()
	}
	theme := fs.String("theme", "", "CLI theme: default or minimal")
	projectType := fs.String("project-type", "", "project type, e.g. backend-service")
	language := fs.String("language", "", "language, e.g. go")
	framework := fs.String("framework", "", "framework, e.g. rest-api")
	caps := fs.String("capabilities", "", "comma-separated capability ids")
	answersFile := fs.String("answers", "", "path to an answers file")
	dirFlag := fs.String("dir", "", "target directory, combined with the project name (e.g. -dir ~/Projects my-app); can't be combined with a path-like project name")
	noColor := fs.Bool("no-color", false, "disable color output")
	verbose := fs.Bool("verbose", false, "print diagnostic logging (plugin spawn/timing) to stderr")
	fs.BoolVar(verbose, "v", false, "shorthand for -verbose")
	yes := fs.Bool("yes", false, "skip plugin-execution confirmation prompts (implied by --answers and non-interactive runs)")
	fs.Parse(rest)

	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "error: unexpected argument(s): %s\n", strings.Join(fs.Args(), " "))
		fs.Usage()
		exit(2)
	}

	noColorEnv := os.Getenv("NO_COLOR") != ""
	interactive := *answersFile == "" && positional == "" && *theme == "" && *projectType == "" && *language == "" && *framework == "" && *caps == "" && *dirFlag == ""

	var a config.Answers
	var err error

	switch {
	case *answersFile != "":
		if positional != "" {
			fs.Usage()
			fmt.Fprintln(os.Stderr, "error: --answers and a positional project name can't be combined (the project name belongs in the answers file)")
			exit(2)
		}
		if *dirFlag != "" {
			fs.Usage()
			fmt.Fprintln(os.Stderr, "error: --answers and --dir can't be combined (the location belongs in the answers file's projectName)")
			exit(2)
		}
		a, err = prompt.ParseAnswersFile(*answersFile)
	case !interactive:
		if *theme == "" {
			// Non-interactive runs follow the same theme precedence as
			// the wizard: LUMO_THEME > persisted config > default.
			cfg, _ := prompt.LoadConfig()
			*theme = prompt.ResolveThemeName("", cfg.Theme)
		}
		if *dirFlag != "" {
			if positional == "" {
				fs.Usage()
				fmt.Fprintln(os.Stderr, "error: --dir requires a project name")
				exit(2)
			}
			if prompt.IsPathLike(positional) {
				fs.Usage()
				fmt.Fprintln(os.Stderr, "error: --dir and a path-like project name can't be combined — pass a bare name with --dir, or a path with neither")
				exit(2)
			}
			positional = filepath.Join(*dirFlag, positional)
		}
		a = config.Answers{
			ProjectName:  positional,
			Theme:        *theme,
			ProjectType:  *projectType,
			Language:     *language,
			Framework:    *framework,
			Capabilities: splitCSV(*caps),
		}
	default:
		// Banner uses the resolved non-interactive theme name
		// (LUMO_THEME > persisted > default); the wizard's own theme
		// question can still change it.
		cfg, _ := prompt.LoadConfig()
		bannerTheme := prompt.ResolveThemeName("", cfg.Theme)
		prompt.Banner(os.Stdout, prompt.GetTheme(bannerTheme, *noColor || noColorEnv))

		// The wizard's menus are built from what's actually installed
		// (registry-driven — see prompt.WizardSpec), so discovery runs
		// first and a discovery failure is fatal before any question is
		// asked.
		reg := registry.New(pluginDirs()...)
		discovered, discoverErr := reg.Discover()
		if discoverErr != nil {
			prompt.ErrorScreen(os.Stdout, prompt.GetTheme(bannerTheme, *noColor || noColorEnv), discoverErr)
			exit(1)
		}
		a, err = prompt.RunWizard(os.Stdout, wizardSpecFromDiscovery(discovered), version)
	}

	t := prompt.GetTheme(a.Theme, *noColor || noColorEnv)

	if err != nil {
		if err == prompt.ErrCancelled {
			fmt.Fprintln(os.Stdout, t.Info("cancelled"))
			exit(130)
		}
		prompt.ErrorScreen(os.Stdout, t, err)
		exit(1)
	}

	// The positional argument may be a bare project name or a target
	// path (relative, absolute, or ~-prefixed). resolveTargetPath
	// separates the two concerns: the project name is the final path
	// component (what templates name the module/service after), and the
	// target dir is the full resolved location.
	targetDir := ""
	if a.ProjectName != "" {
		dir, name, err := resolveTargetPath(a.ProjectName)
		if err != nil {
			prompt.ErrorScreen(os.Stdout, t, err)
			exit(1)
		}
		a.ProjectName = name
		targetDir = dir
	}
	if targetDir == "" {
		targetDir, err = filepath.Abs(a.ProjectName)
		if err != nil {
			prompt.ErrorScreen(os.Stdout, t, err)
			exit(1)
		}
	}

	// a.ProjectName is now just the bare name (resolveTargetPath already
	// split any path off into targetDir above) — the strict character-set
	// check deferred by ValidateShape/ParseAnswersFile runs here, once,
	// for every path into cmdNew (wizard, --answers, and plain flags
	// alike), matching ValidateShape's own doc comment.
	if err := a.Validate(); err != nil {
		prompt.ErrorScreen(os.Stdout, t, err)
		exit(1)
	}

	// Refuse to generate into a directory that already has content —
	// plugins write files unconditionally, so running `lumo new
	// existing-project` (or re-running it) would silently overwrite.
	if info, statErr := os.Stat(targetDir); statErr == nil {
		if !info.IsDir() {
			prompt.ErrorScreen(os.Stdout, t, fmt.Errorf("target %s is a file, not a directory", targetDir))
			exit(1)
		}
		entries, readErr := os.ReadDir(targetDir)
		if readErr != nil {
			prompt.ErrorScreen(os.Stdout, t, readErr)
			exit(1)
		}
		if len(entries) > 0 {
			prompt.ErrorScreen(os.Stdout, t, fmt.Errorf("target directory %s already exists and is not empty (generating would overwrite existing files)", targetDir))
			exit(1)
		}
	} else if !os.IsNotExist(statErr) {
		prompt.ErrorScreen(os.Stdout, t, statErr)
		exit(1)
	}

	var logger diag.Logger = diag.NoopLogger{}
	if *verbose {
		logger = diag.WriterLogger{W: os.Stderr}
	}

	reg := registry.New(pluginDirs()...)

	nonInteractive := *yes || *answersFile != ""
	if err := confirmPluginTrust(reg, a, nonInteractive); err != nil {
		prompt.ErrorScreen(os.Stdout, t, err)
		exit(1)
	}

	host := plugin.NewHost()
	host.Logger = logger
	if *verbose {
		// Surface plugin stderr (protocol-reserved for plugin logs) so a
		// hung or failing plugin's own output is visible — it's otherwise
		// discarded by default (see core/plugin.Host.Stderr).
		host.Stderr = os.Stderr
	}
	eng := engine.New(reg, host)
	eng.Logger = logger

	// A one-line plan of what will be created, then the persistent
	// step-based progress bar (progress.go): one row per phase as it
	// begins, filling as the engine reports each phase's completion.
	// The same output works in CI and under pipes: it degrades to
	// plain arrow lines with no escape codes.
	plan := fmt.Sprintf("Creating %s — %s · %s · %s", a.ProjectName, a.ProjectType, a.Language, a.Framework)
	if len(a.Capabilities) > 0 {
		plan += " · " + strings.Join(a.Capabilities, ", ")
	}
	prompt.SetTitle("Lumo — generating " + a.ProjectName)
	fmt.Fprintln(os.Stdout, "  "+prompt.SummaryLine(t, plan))

	var pg *prompt.ProgressGroup
	eng.Progress = func(phase string, done bool) {
		if done {
			if pg != nil {
				pg.Done(phase)
			}
			return
		}
		if pg == nil {
			pg = prompt.NewProgressGroup(os.Stdout, t, a.Capabilities)
		}
		pg.Start(phase)
	}

	summary, err := eng.Run(targetDir, a)
	if err != nil {
		if pg != nil {
			pg.Fail()
		}
		prompt.ErrorScreen(os.Stdout, t, err)
		exit(1)
	}
	var elapsed time.Duration
	if pg != nil {
		elapsed = pg.Finish()
	}

	prompt.SuccessScreen(os.Stdout, t, a.ProjectName, summary, elapsed)
}

func cmdPlugins(args []string) {
	if len(args) == 0 {
		pluginsUsage()
		exit(1)
	}
	switch args[0] {
	case "list":
		cmdPluginsList(args[1:])
	case "validate":
		cmdPluginsValidate(args[1:])
	default:
		pluginsUsage()
		exit(1)
	}
}

func pluginsUsage() {
	fmt.Fprintln(os.Stderr, `usage: lumo plugins list
       lumo plugins validate <plugin-dir>`)
}

func cmdPluginsList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "usage: lumo plugins list") }
	fs.Parse(args)

	reg := registry.New(pluginDirs()...)
	found, issues, err := reg.DiscoverWithIssues()
	if err != nil {
		prompt.ErrorScreen(os.Stdout, prompt.GetTheme("default", os.Getenv("NO_COLOR") != ""), err)
		exit(1)
	}
	for _, issue := range issues {
		fmt.Fprintf(os.Stderr, "skipped %s: %v\n", issue.Path, issue.Err)
	}
	if len(found) == 0 {
		fmt.Println("no plugins found")
		return
	}
	for _, p := range found {
		fmt.Printf("%s\t%s\t%s\t%s\n", p.Manifest.Name, p.Manifest.Kind, p.Manifest.Version, p.Manifest.DisplayName)
	}
}

// cmdPluginsValidate checks a single plugin directory before release:
// its plugin.json must parse and pass Manifest.Validate(), and its
// entrypoint binary must be spawnable and pass the plugin.initialize
// identity/protocol cross-check against the on-disk manifest (a stale
// or swapped binary fails here, the same way it would during `new`).
// Exits 0 on success, 1 on any failure.
func cmdPluginsValidate(args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: lumo plugins validate <plugin-dir>

Checks a plugin directory before shipping it: plugin.json must parse
and pass Manifest.Validate(), and the entrypoint binary must spawn and
pass the plugin.initialize identity/protocol cross-check against the
on-disk manifest (catching a stale or swapped binary). Exit 0 = valid.`)
	}
	fs.Parse(args)

	if fs.NArg() != 1 {
		fs.Usage()
		exit(1)
	}

	t := prompt.GetTheme("default", os.Getenv("NO_COLOR") != "")
	dir := fs.Arg(0)

	p, ok, err := registry.LoadPluginDir(dir)
	if err != nil {
		prompt.ErrorScreen(os.Stdout, t, fmt.Errorf("invalid plugin in %s: %w", dir, err))
		exit(1)
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "no plugin.json found in %s\n", dir)
		exit(1)
	}

	host := plugin.NewHost()
	if err := host.Validate(p.EntrypointPath, p.Manifest.Name, sdk.ProtocolVersion); err != nil {
		prompt.ErrorScreen(os.Stdout, t, fmt.Errorf("plugin %q failed validation: %w", p.Manifest.Name, err))
		exit(1)
	}

	fmt.Println(t.Success(fmt.Sprintf("plugin %s (%s) v%s: valid", p.Manifest.Name, p.Manifest.Kind, p.Manifest.Version)))
}

// printEmbeddedStatus reports whether this binary has embedded plugin
// assets at all, and whether the embedded fallback is the thing
// actually serving plugins for this run (i.e. dirs, as already computed
// by pluginDirs(), ends in the embedded cache directory's two
// subdirectories — see pluginDirs()'s comment on why it's two entries,
// not the bare cache root).
func printEmbeddedStatus(t prompt.Theme, dirs []string) {
	if !embedded.Available() {
		fmt.Println(t.Dim("  no plugin assets embedded in this binary (a dev build without `make build`'s staging step)"))
		return
	}
	fmt.Println(t.Success("plugin assets are embedded in this binary"))

	cacheDir, err := embeddedCacheDir()
	if err != nil || len(dirs) < 2 {
		return
	}
	last, secondLast := dirs[len(dirs)-1], dirs[len(dirs)-2]
	if last == filepath.Join(cacheDir, "plugins", "builtin") && secondLast == filepath.Join(cacheDir, "templates") {
		fmt.Println(t.Info("  in use: no sibling plugin directories were found, so plugins were self-extracted to " + cacheDir))
	} else {
		fmt.Println(t.Dim("  not in use: sibling plugin directories were found, so the embedded fallback wasn't needed"))
	}
}

// cmdDoctor runs a small set of local health checks — plugin directory
// resolution and manifest validity — and reports a pass/fail summary
// with actionable hints. It never spawns any plugin subprocess (that
// happens only during `new`); it only exercises discovery, the same
// fail-fast surface `plugins list` uses.
func cmdDoctor(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "usage: lumo doctor") }
	fs.Parse(args)

	t := prompt.GetTheme("default", os.Getenv("NO_COLOR") != "")
	ok := true

	dirs := pluginDirs()
	fmt.Println(t.Header("Plugin directories:"))
	for _, d := range dirs {
		abs, err := filepath.Abs(d)
		if err != nil {
			abs = d
		}
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			fmt.Println(t.Success(abs))
		} else {
			fmt.Println(t.Dim("  " + abs + " (not found, skipped)"))
		}
	}

	fmt.Println()
	fmt.Println(t.Header("Embedded fallback:"))
	printEmbeddedStatus(t, dirs)

	reg := registry.New(dirs...)
	found, issues, err := reg.DiscoverWithIssues()
	if err != nil {
		fmt.Println(t.Failure("discovery failed: " + err.Error()))
		exit(1)
	}

	fmt.Println()
	fmt.Println(t.Header("Plugins:"))
	if len(found) == 0 {
		fmt.Println(t.Dim("  none discovered"))
		ok = false
	}
	for _, p := range found {
		fmt.Println(t.Success(fmt.Sprintf("%s (%s) v%s", p.Manifest.Name, p.Manifest.Kind, p.Manifest.Version)))
	}
	for _, issue := range issues {
		fmt.Println(t.Failure(fmt.Sprintf("%s: %v", issue.Path, issue.Err)))
		ok = false
	}

	fmt.Println()
	if ok {
		fmt.Println(t.Success("doctor: all checks passed"))
		return
	}
	fmt.Println(t.Failure("doctor: found issues"))
	fmt.Println(t.Dim("  hint: check plugin.json files against docs/plugins/authoring.md / docs/templates/authoring.md, or set LUMO_PLUGIN_DIRS to point at the right directories."))
	exit(1)
}

// statusUsage prints the usage/help text for `lumo status`.
func statusUsage() {
	fmt.Fprintln(os.Stderr, `usage: lumo status [flags]

Shows the current repo/project, whether Git is initialized, and whether
GitHub and SonarQube are reachable/configured.`)
}

// cmdStatus reports the current project/Git/GitHub/SonarQube state.
// --offline skips every network call (GitHub's `gh auth status`, the
// SonarQube HTTP check, and the govulncheck dependency-vulnerability
// scan, which fetches the Go vulnerability database) as well as
// secretstore.New() — the SonarQube token lookup is part of the network
// path, not the offline one — so `lumo status --offline` never touches
// the network or the OS secret store. Network calls are announced on
// stderr (not stdout) so piped or parsed stdout output stays clean.
func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	fs.Usage = statusUsage
	offline := fs.Bool("offline", false, "skip GitHub/SonarQube network checks")
	verbose := fs.Bool("verbose", false, "print phase-by-phase connector logging to stderr")
	fs.BoolVar(verbose, "v", false, "shorthand for -verbose")
	fs.Parse(args)

	cfg, _ := prompt.LoadConfig()
	themeName := prompt.ResolveThemeName("", cfg.Theme)
	t := prompt.GetTheme(themeName, os.Getenv("NO_COLOR") != "")

	var logger diag.Logger = diag.NoopLogger{}
	if *verbose {
		logger = diag.WriterLogger{W: os.Stderr}
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	projectName := filepath.Base(cwd)
	fmt.Println(t.Header("Project:"))
	fmt.Println(t.Success(fmt.Sprintf("%s (%s)", projectName, cwd)))
	fmt.Println()

	fmt.Println(t.Header("Git:"))
	gitCmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	gitCmd.Dir = cwd
	if out, gitErr := gitCmd.Output(); gitErr == nil && strings.TrimSpace(string(out)) == "true" {
		fmt.Println(t.Success("initialized"))
	} else {
		fmt.Println(t.Failure("not initialized"))
	}
	fmt.Println()

	fmt.Println(t.Header("GitHub:"))
	if *offline {
		fmt.Println(t.Dim("offline — not checked"))
	} else {
		fmt.Fprintln(os.Stderr, "→ network: GitHub API (gh auth status)")
		gh := connector.NewGitHubConnector(connector.ExecCmdRunner{})
		res, err := connector.RunEngine(gh, logger)
		printConnectorResult(t, res, err)
	}
	fmt.Println()

	fmt.Println(t.Header("SonarQube:"))
	switch {
	case *offline:
		fmt.Println(t.Dim("offline — not checked"))
	case cfg.SonarQubeURL == "":
		fmt.Println(t.Dim("not configured (see 'lumo config set sonarqube-url')"))
	default:
		store, native, storeErr := secretstore.New()
		if storeErr != nil {
			fmt.Println(t.Failure("error reading token: " + storeErr.Error()))
			break
		}
		if !native {
			fmt.Fprintln(os.Stderr, "warning: SonarQube token is stored in an unencrypted local file (no OS secret store available)")
		}
		token, found, getErr := store.Get("sonarqube-token")
		if getErr != nil {
			fmt.Println(t.Failure("error reading token: " + getErr.Error()))
			break
		}
		if !found {
			fmt.Println(t.Dim("not configured (see 'lumo config set sonarqube-token')"))
			break
		}
		fmt.Fprintln(os.Stderr, "→ network: SonarQube ("+cfg.SonarQubeURL+"/api/system/status)")
		sq := connector.NewSonarQubeConnector(cfg.SonarQubeURL, token, nil)
		res, err := connector.RunEngine(sq, logger)
		printConnectorResult(t, res, err)
	}

	fmt.Println()
	fmt.Println(t.Header("Dependency vulnerabilities (Go):"))
	switch {
	case *offline:
		fmt.Println(t.Dim("offline — not checked"))
	default:
		if _, statErr := os.Stat(filepath.Join(cwd, "go.mod")); statErr != nil {
			fmt.Println(t.Dim("skipped (no go.mod in current directory)"))
		} else if _, lookErr := exec.LookPath("go"); lookErr != nil {
			fmt.Println(t.Dim("skipped (go toolchain not found on PATH)"))
		} else {
			fmt.Fprintln(os.Stderr, "→ network: Go vulnerability database (govulncheck)")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, "go", "run", "golang.org/x/vuln/cmd/govulncheck@latest", "./...").CombinedOutput()
			switch {
			case err != nil && !strings.Contains(string(out), "vulnerabilities"):
				fmt.Println(t.Dim("skipped (govulncheck unavailable: " + firstLine(string(out)) + ")"))
			case strings.Contains(string(out), "No vulnerabilities found"), strings.Contains(string(out), "0 vulnerabilities"):
				fmt.Println(t.Success("no known vulnerabilities"))
			default:
				fmt.Println(t.Failure("vulnerabilities found — run 'go run golang.org/x/vuln/cmd/govulncheck@latest ./...' for details"))
			}
		}
	}
}

// printConnectorResult renders a connector's phase-engine outcome: a
// PhaseError (or any other error) as a failure line naming what went
// wrong, otherwise the result's own Connected/Detail.
func printConnectorResult(t prompt.Theme, res connector.Result, err error) {
	if err != nil {
		fmt.Println(t.Failure(err.Error()))
		return
	}
	if res.Connected {
		fmt.Println(t.Success(res.Detail))
	} else {
		fmt.Println(t.Failure(res.Detail))
	}
}

// firstLine returns the text up to (not including) the first newline in
// s, or all of s if it has no newline. Used to keep a subprocess's
// (potentially multi-line) error output to a single summary line.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func configUsage() {
	fmt.Fprintln(os.Stderr, `usage: lumo config get theme
       lumo config set theme <default|minimal>
       lumo config get projects-dir
       lumo config set projects-dir <path>
       lumo config get sonarqube-url
       lumo config set sonarqube-url <url>
       lumo config set sonarqube-token   (interactive prompt; never accepts the token as an argument)`)
}

func cmdConfig(args []string) {
	if len(args) < 2 || (args[0] != "get" && args[0] != "set") {
		configUsage()
		exit(1)
	}
	key, action := args[1], args[0]
	switch key {
	case "theme":
		if action == "get" {
			cmdConfigGetTheme()
		} else {
			cmdConfigSetTheme(args[2:])
		}
	case "projects-dir":
		if action == "get" {
			cmdConfigGetProjectsDir()
		} else {
			cmdConfigSetProjectsDir(args[2:])
		}
	case "sonarqube-url":
		if action == "get" {
			cmdConfigGetSonarQubeURL()
		} else {
			cmdConfigSetSonarQubeURL(args[2:])
		}
	case "sonarqube-token":
		if action == "get" {
			fmt.Fprintln(os.Stderr, "error: sonarqube-token cannot be read back (write-only; use 'lumo status' to check it's configured)")
			exit(2)
		}
		cmdConfigSetSonarQubeToken()
	default:
		configUsage()
		exit(1)
	}
}

func cmdConfigGetTheme() {
	cfg, err := prompt.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	fmt.Println(cfg.Theme)
}

func cmdConfigSetTheme(rest []string) {
	if len(rest) < 1 {
		configUsage()
		exit(1)
	}
	name := rest[0]
	if !isValidThemeName(name) {
		fmt.Fprintf(os.Stderr, "unknown theme %q (want one of: %s)\n", name, strings.Join(prompt.ThemeNames(), ", "))
		exit(1)
	}
	cfg, err := prompt.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	cfg.Theme = name
	if err := prompt.SaveConfig(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
}

func cmdConfigGetProjectsDir() {
	cfg, err := prompt.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	fmt.Println(cfg.DefaultProjectsDir)
}

func cmdConfigSetProjectsDir(rest []string) {
	if len(rest) < 1 || rest[0] == "" {
		configUsage()
		exit(1)
	}
	cfg, err := prompt.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	cfg.DefaultProjectsDir = rest[0]
	if err := prompt.SaveConfig(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
}

func cmdConfigGetSonarQubeURL() {
	cfg, err := prompt.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	fmt.Println(cfg.SonarQubeURL)
}

func cmdConfigSetSonarQubeURL(rest []string) {
	if len(rest) < 1 || rest[0] == "" {
		configUsage()
		exit(1)
	}
	cfg, err := prompt.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	cfg.SonarQubeURL = rest[0]
	if err := prompt.SaveConfig(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
}

// cmdConfigSetSonarQubeToken reads the token interactively (never as a
// CLI argument, to avoid shell-history/process-list leakage — spec
// Section 2.1) and stores it via secretstore, warning if the OS has no
// native secret store reachable and Lumo is falling back to an
// unencrypted local file.
func cmdConfigSetSonarQubeToken() {
	fmt.Print("SonarQube token: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	token := strings.TrimSpace(line)
	if token == "" {
		fmt.Fprintln(os.Stderr, "error: token cannot be empty")
		exit(1)
	}

	store, native, err := secretstore.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	if !native {
		fmt.Fprintln(os.Stderr, "warning: no OS secret store available — the token will be stored in a local file, unencrypted at rest")
	}
	if err := store.Set("sonarqube-token", token); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		exit(1)
	}
	fmt.Println("SonarQube token saved.")
}

// pluginTrustKey identifies a specific plugin build for consent tracking:
// name+version+resolved path, so a plugin relocated to a different path,
// or bumped to a new version, requires consent again; swapping the
// binary at the same path with the same declared name/version is not
// detected by this key alone — see SECURITY.md's "installing a plugin is
// consent to run it" model and spec Section 2.2.
func pluginTrustKey(p registry.Plugin) string {
	return p.Manifest.Name + "@" + p.Manifest.Version + "@" + p.EntrypointPath
}

func isApproved(cfg prompt.Config, key string) bool {
	for _, k := range cfg.ApprovedPlugins {
		if k == key {
			return true
		}
	}
	return false
}

// confirmPluginTrust resolves every plugin a.ProjectType/Language/
// Framework/Capabilities will run and, for any not already approved,
// prompts for confirmation (skipped entirely when yes is true — the
// existing non-interactive contract for --answers/CI runs, ADR-0007).
// Approvals are persisted to prompt.Config.ApprovedPlugins so the same
// plugin version+path never re-prompts.
func confirmPluginTrust(reg *registry.Registry, a config.Answers, yes bool) error {
	if yes {
		return nil
	}

	var toConfirm []registry.Plugin
	tmpl, err := reg.ResolveTemplate(a.ProjectType, a.Language, a.Framework)
	if err != nil {
		return err
	}
	toConfirm = append(toConfirm, tmpl)
	for _, capID := range a.Capabilities {
		capPlugin, err := reg.ResolveCapability(capID)
		if err != nil {
			return err
		}
		toConfirm = append(toConfirm, capPlugin)
	}

	cfg, err := prompt.LoadConfig()
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)
	changed := false
	for _, p := range toConfirm {
		key := pluginTrustKey(p)
		if isApproved(cfg, key) {
			continue
		}
		fmt.Printf("About to run plugin %q v%s (%s). Continue? [y/N] ", p.Manifest.Name, p.Manifest.Version, p.EntrypointPath)
		line, _ := reader.ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer != "y" && answer != "yes" {
			return fmt.Errorf("declined to run plugin %q", p.Manifest.Name)
		}
		cfg.ApprovedPlugins = append(cfg.ApprovedPlugins, key)
		changed = true
	}
	if changed {
		if err := prompt.SaveConfig(cfg); err != nil {
			return err
		}
	}
	return nil
}

func isValidThemeName(name string) bool {
	for _, n := range prompt.ThemeNames() {
		if n == name {
			return true
		}
	}
	return false
}
