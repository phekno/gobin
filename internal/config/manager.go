package config

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// ErrSaveFailed indicates the config was updated in memory but could not be
// persisted to disk (e.g., read-only filesystem like a Kubernetes ConfigMap).
var ErrSaveFailed = errors.New("config save failed")

// Manager holds the live config with safe concurrent access and hot-reload support.
type Manager struct {
	mu       sync.RWMutex
	current  *Config
	filePath string
}

// NewManager creates a config manager with the given initial config and file path.
func NewManager(path string, cfg *Config) *Manager {
	return &Manager{current: cfg, filePath: path}
}

// Get returns the current config. Safe for concurrent use.
func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// FilePath returns the path to the config file.
func (m *Manager) FilePath() string {
	return m.filePath
}

// Update validates, merges redacted fields, saves to disk, and swaps the live config.
// The in-memory config is always updated on success or disk-save failure.
// Returns an ErrSaveFailed-wrapped error if the config was applied in memory
// but could not be persisted (e.g., read-only filesystem).
func (m *Manager) Update(edited *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Restore any redacted placeholder values from the current config
	MergeRedacted(edited, m.current)

	applyDefaults(edited)

	if err := Validate(edited); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := Save(m.filePath, edited); err != nil {
		// Still apply in memory so the change takes effect for this session
		m.current = edited
		slog.Warn("config updated in memory only (disk save failed)", "error", err, "path", m.filePath)
		return fmt.Errorf("%w: %v", ErrSaveFailed, err)
	}

	m.current = edited
	slog.Info("config updated and reloaded", "path", m.filePath)
	return nil
}
