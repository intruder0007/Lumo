package prompt

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config is the CLI's small persisted local config — the last-chosen
// theme and the last-used project location. Only the interactive
// wizard path writes it (see ADR-0007): --theme/--answers/--dir runs
// never mutate persisted state, so scripted/CI usage stays
// side-effect-free.
type Config struct {
	Theme string `json:"theme,omitempty"`
	// DefaultProjectsDir is the last directory the interactive wizard's
	// location step was pointed at — offered as that step's editable
	// pre-fill on the next run (never silently applied without asking;
	// see wizard.go's stepLocation).
	DefaultProjectsDir string `json:"defaultProjectsDir,omitempty"`
	// SonarQubeURL is the configured SonarQube (self-hosted or
	// SonarCloud) server to check in `lumo status`. The auth token is
	// never stored here — see core/secretstore, keyed by "sonarqube-token".
	SonarQubeURL string `json:"sonarQubeURL,omitempty"`
	// ApprovedPlugins records plugins the user has already consented to
	// run, as "name@version@entrypointPath" entries, so `lumo new`/`lumo
	// plugins validate` only prompts once per plugin version+location
	// (see confirmPluginTrust in main.go).
	ApprovedPlugins []string `json:"approvedPlugins,omitempty"`
}

// configPath returns the path to the CLI's config file, using the OS's
// standard per-user config directory (%AppData% on Windows,
// ~/Library/Application Support on macOS, $XDG_CONFIG_HOME or ~/.config
// on Linux).
func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lumo", "config.json"), nil
}

// LoadConfig reads the persisted config. A missing file is not an error
// — it returns a zero-value Config (no persisted theme yet).
func LoadConfig() (Config, error) {
	var c Config
	path, err := configPath()
	if err != nil {
		return c, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	return c, nil
}

// SaveConfig writes the config, creating its directory if needed.
func SaveConfig(c Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
