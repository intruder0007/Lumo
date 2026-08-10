package kernel

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// ConfigStore holds persistent, user-authored settings (theme choice,
// offline-mode default, per-domain preferences). It survives across
// runs, backed by a single JSON file on disk. Config is read by any
// domain but written only through explicit user action — no domain
// infers and silently writes a config value.
type ConfigStore struct {
	mu   sync.RWMutex
	path string
	data map[string]string
}

// NewConfigStore loads settings from path, if it exists. A missing file
// is not an error — it means no settings have been saved yet.
func NewConfigStore(path string) (*ConfigStore, error) {
	c := &ConfigStore{path: path, data: make(map[string]string)}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, fmt.Errorf("kernel: reading config %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &c.data); err != nil {
		return nil, fmt.Errorf("kernel: parsing config %s: %w", path, err)
	}
	return c, nil
}

// Get returns key's persisted value. ok is false if key was never set.
func (c *ConfigStore) Get(key string) (value string, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, ok = c.data[key]
	return value, ok
}

// Set persists key=value to disk immediately, replacing any prior value.
func (c *ConfigStore) Set(key, value string) error {
	c.mu.Lock()
	c.data[key] = value
	snapshot := make(map[string]string, len(c.data))
	for k, v := range c.data {
		snapshot[k] = v
	}
	c.mu.Unlock()

	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("kernel: encoding config: %w", err)
	}
	if err := os.WriteFile(c.path, raw, 0o644); err != nil {
		return fmt.Errorf("kernel: writing config %s: %w", c.path, err)
	}
	return nil
}

// SessionStore holds ephemeral, in-memory, TUI-only state (which panel
// has focus, scroll position, last-run command). It is discarded on
// exit and never persisted — domains needing persistence use
// ConfigStore instead.
type SessionStore struct {
	mu   sync.RWMutex
	data map[string]any
}

// NewSessionStore returns an empty SessionStore.
func NewSessionStore() *SessionStore {
	return &SessionStore{data: make(map[string]any)}
}

// Get returns key's current in-memory value. ok is false if key was
// never set this session.
func (s *SessionStore) Get(key string) (value any, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok = s.data[key]
	return value, ok
}

// Set stores value under key for the lifetime of this process. It is
// never written to disk.
func (s *SessionStore) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}
