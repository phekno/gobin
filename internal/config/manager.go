package config

import (
	"fmt"
	"log/slog"
	"sync"
)

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
		return fmt.Errorf("saving config: %w", err)
	}

	m.current = edited
	slog.Info("config updated and reloaded", "path", m.filePath)
	return nil
}
