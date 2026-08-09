package prompt

import (
	"runtime"
	"testing"
)

// withTempConfigDir redirects os.UserConfigDir() to t.TempDir() for the
// duration of the test, so LoadConfig/SaveConfig never touch the real
// user's config file.
func withTempConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("AppData", dir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	withTempConfigDir(t)

	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig on a missing file should not error, got: %v", err)
	}
	if got.Theme != "" {
		t.Errorf("LoadConfig with no saved file: got Theme=%q, want empty", got.Theme)
	}
	if got.DefaultProjectsDir != "" {
		t.Errorf("LoadConfig with no saved file: got DefaultProjectsDir=%q, want empty", got.DefaultProjectsDir)
	}

	if err := SaveConfig(Config{Theme: "minimal", DefaultProjectsDir: `C:\Users\me\Projects`}); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	got, err = LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig after save: %v", err)
	}
	if got.Theme != "minimal" {
		t.Errorf("LoadConfig after SaveConfig(Theme: \"minimal\"): got %q, want %q", got.Theme, "minimal")
	}
	if want := `C:\Users\me\Projects`; got.DefaultProjectsDir != want {
		t.Errorf("LoadConfig after SaveConfig(DefaultProjectsDir): got %q, want %q", got.DefaultProjectsDir, want)
	}
}

func TestConfigRoundTripSonarQubeAndApprovedPlugins(t *testing.T) {
	withTempConfigDir(t)

	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig on a missing file should not error, got: %v", err)
	}
	if got.SonarQubeURL != "" {
		t.Errorf("LoadConfig with no saved file: got SonarQubeURL=%q, want empty", got.SonarQubeURL)
	}
	if len(got.ApprovedPlugins) != 0 {
		t.Errorf("LoadConfig with no saved file: got ApprovedPlugins=%v, want empty", got.ApprovedPlugins)
	}

	cfg := Config{
		SonarQubeURL:    "https://sonar.example.com",
		ApprovedPlugins: []string{"git-init@1.0.0@/path/to/git-init"},
	}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	got, err = LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig after save: %v", err)
	}
	if got.SonarQubeURL != cfg.SonarQubeURL {
		t.Errorf("LoadConfig after SaveConfig: got SonarQubeURL=%q, want %q", got.SonarQubeURL, cfg.SonarQubeURL)
	}
	if len(got.ApprovedPlugins) != 1 || got.ApprovedPlugins[0] != cfg.ApprovedPlugins[0] {
		t.Errorf("LoadConfig after SaveConfig: got ApprovedPlugins=%v, want %v", got.ApprovedPlugins, cfg.ApprovedPlugins)
	}
}
