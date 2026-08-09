package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/intruder0007/Lumo/core/registry"
	sdk "github.com/intruder0007/Lumo/sdk/go/sdk"
)

func TestHasAnyPluginFalseForEmptyOrMissingDirs(t *testing.T) {
	empty := t.TempDir()
	missing := filepath.Join(empty, "does-not-exist")
	if hasAnyPlugin([]string{empty, missing}) {
		t.Fatal("expected hasAnyPlugin to be false for an empty dir and a missing dir")
	}
}

func TestHasAnyPluginFalseForSubdirWithoutManifest(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "not-a-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if hasAnyPlugin([]string{dir}) {
		t.Fatal("expected hasAnyPlugin to be false when no subdirectory has a plugin.json")
	}
}

func TestHasAnyPluginTrueWhenManifestPresent(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "some-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasAnyPlugin([]string{dir}) {
		t.Fatal("expected hasAnyPlugin to be true when a subdirectory has a plugin.json")
	}
}

// TestPluginDirsOverrideSkipsFallback verifies that an explicit
// LUMO_PLUGIN_DIRS always wins, even pointing at an empty directory —
// the embedded fallback must never silently override deliberate intent.
func TestPluginDirsOverrideSkipsFallback(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("LUMO_PLUGIN_DIRS", empty)

	dirs := pluginDirs()
	if len(dirs) != 1 || dirs[0] != empty {
		t.Fatalf("expected pluginDirs() to return exactly the override %q unchanged, got %v", empty, dirs)
	}
}

func TestEmbeddedCacheDirIsVersionScoped(t *testing.T) {
	dir, err := embeddedCacheDir()
	if err != nil {
		t.Fatalf("embeddedCacheDir: %v", err)
	}
	if filepath.Base(dir) != version {
		t.Fatalf("expected cache dir %q to end with the current version %q", dir, version)
	}
	if filepath.Base(filepath.Dir(dir)) != "lumo" {
		t.Fatalf("expected cache dir %q to be under a 'lumo' directory", dir)
	}
}

func TestResolveTargetPathBareName(t *testing.T) {
	dir, name, err := resolveTargetPath("myapp")
	if err != nil {
		t.Fatalf("resolveTargetPath(bare): %v", err)
	}
	if name != "myapp" {
		t.Fatalf("expected name myapp, got %q", name)
	}
	if want := filepath.Join(mustAbs(t, "."), "myapp"); dir != want {
		t.Fatalf("expected dir %q, got %q", want, dir)
	}
}

func TestResolveTargetPathRelative(t *testing.T) {
	dir, name, err := resolveTargetPath("./a/b/app")
	if err != nil {
		t.Fatalf("resolveTargetPath(relative): %v", err)
	}
	if name != "app" {
		t.Fatalf("expected name app, got %q", name)
	}
	if want := filepath.Join(mustAbs(t, "."), "a", "b", "app"); dir != want {
		t.Fatalf("expected dir %q, got %q", want, dir)
	}
}

func TestResolveTargetPathAbsolute(t *testing.T) {
	abs := filepath.Join(mustAbs(t, "."), "deep", "app")
	dir, name, err := resolveTargetPath(abs)
	if err != nil {
		t.Fatalf("resolveTargetPath(absolute): %v", err)
	}
	if name != "app" {
		t.Fatalf("expected name app, got %q", name)
	}
	if dir != abs {
		t.Fatalf("expected dir %q, got %q", abs, dir)
	}
}

func TestResolveTargetPathHomeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	dir, name, err := resolveTargetPath("~/code/app")
	if err != nil {
		t.Fatalf("resolveTargetPath(~): %v", err)
	}
	if name != "app" {
		t.Fatalf("expected name app, got %q", name)
	}
	if want := filepath.Join(home, "code", "app"); dir != want {
		t.Fatalf("expected dir %q, got %q", want, dir)
	}

	if _, _, err := resolveTargetPath("~"); err == nil {
		t.Fatal("expected ~ alone to be rejected (not a project directory)")
	}
}

func TestResolveTargetPathRejectsNonDirectories(t *testing.T) {
	for _, p := range []string{"", ".", "./", "..", "/", "~/"} {
		if _, _, err := resolveTargetPath(p); err == nil {
			t.Fatalf("expected %q to be rejected", p)
		}
	}
}

func TestResolveTargetPathKeepsBadNameForValidation(t *testing.T) {
	_, name, err := resolveTargetPath("./x/2bad")
	if err != nil {
		t.Fatalf("resolveTargetPath(bad name): %v", err)
	}
	if name != "2bad" {
		t.Fatalf("expected name 2bad, got %q", name)
	}
}

func TestPluginTrustKeyIncludesNameVersionAndPath(t *testing.T) {
	p := registry.Plugin{
		Manifest:       sdk.Manifest{Name: "git-init", Version: "1.0.0"},
		EntrypointPath: "/plugins/git-init/git-init",
	}
	got := pluginTrustKey(p)
	want := "git-init@1.0.0@/plugins/git-init/git-init"
	if got != want {
		t.Errorf("pluginTrustKey = %q, want %q", got, want)
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
