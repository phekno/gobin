package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	content := `
general:
  download_dir: /tmp/downloads/incomplete
  complete_dir: /tmp/downloads/complete
  log_level: debug

servers:
  - name: test-server
    host: news.example.com
    port: 563
    tls: true
    username: testuser
    password: testpass
    connections: 10
    priority: 0

categories:
  - name: tv
    dir: TV

downloads:
  max_retries: 5

api:
  listen: 127.0.0.1
  port: 9999
  api_key: test-key
`
	path := writeTemp(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.General.DownloadDir != "/tmp/downloads/incomplete" {
		t.Errorf("DownloadDir = %q", cfg.General.DownloadDir)
	}
	if cfg.General.LogLevel != "debug" {
		t.Errorf("LogLevel = %q", cfg.General.LogLevel)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.Servers))
	}
	if cfg.Servers[0].Host != "news.example.com" {
		t.Errorf("Server host = %q", cfg.Servers[0].Host)
	}
	if cfg.Servers[0].Connections != 10 {
		t.Errorf("Server connections = %d", cfg.Servers[0].Connections)
	}
	if cfg.API.Port != 9999 {
		t.Errorf("API port = %d", cfg.API.Port)
	}
	if cfg.Downloads.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d", cfg.Downloads.MaxRetries)
	}
}

func TestLoadEnvExpansion(t *testing.T) {
	_ = os.Setenv("TEST_GOBIN_HOST", "env.example.com")
	defer func() { _ = os.Unsetenv("TEST_GOBIN_HOST") }()

	content := `
servers:
  - name: env-test
    host: ${TEST_GOBIN_HOST}
    port: 563
    connections: 1
`
	path := writeTemp(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Servers[0].Host != "env.example.com" {
		t.Errorf("Host = %q, want env.example.com", cfg.Servers[0].Host)
	}
}

func TestValidateNoServers(t *testing.T) {
	content := `
general:
  download_dir: /tmp
`
	path := writeTemp(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	// No servers is valid — app starts but can't download
	if cfg.General.DownloadDir != "/tmp" {
		t.Errorf("DownloadDir = %q", cfg.General.DownloadDir)
	}
}

func TestValidateDefaults(t *testing.T) {
	content := `
servers:
  - name: minimal
    host: news.example.com
    port: 563
`
	path := writeTemp(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Check defaults were applied
	if cfg.General.DownloadDir != "/downloads/incomplete" {
		t.Errorf("default DownloadDir = %q", cfg.General.DownloadDir)
	}
	if cfg.API.Port != 8080 {
		t.Errorf("default API port = %d", cfg.API.Port)
	}
	if cfg.Downloads.MaxRetries != 3 {
		t.Errorf("default MaxRetries = %d", cfg.Downloads.MaxRetries)
	}
	if cfg.Servers[0].Connections != 1 {
		t.Errorf("default Connections = %d (should be clamped to 1)", cfg.Servers[0].Connections)
	}
}

func TestLoadCreatesDefaultConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "config.yaml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Default config should have been created
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	// Should have sensible defaults
	if cfg.API.Port != 8080 {
		t.Errorf("default API port = %d", cfg.API.Port)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("expected 1 default server, got %d", len(cfg.Servers))
	}
	if cfg.Servers[0].Port != 563 {
		t.Errorf("default server port = %d", cfg.Servers[0].Port)
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}
	return path
}
