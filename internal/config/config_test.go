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

func TestSave(t *testing.T) {
	cfg := &Config{
		General: General{DownloadDir: "/data/incomplete"},
		API:     API{Port: 9999},
		Servers: []Server{{Name: "s1", Host: "news.test.com", Port: 563}},
	}
	path := filepath.Join(t.TempDir(), "saved.yaml")
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Read back
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if loaded.General.DownloadDir != "/data/incomplete" {
		t.Errorf("DownloadDir = %q", loaded.General.DownloadDir)
	}
	if loaded.API.Port != 9999 {
		t.Errorf("Port = %d", loaded.API.Port)
	}
}

func TestRedacted(t *testing.T) {
	cfg := &Config{
		Servers: []Server{
			{Name: "s1", Host: "news.com", Port: 563, Password: "secret123"},
		},
		API: API{APIKey: "my-api-key"},
		Notifications: Notifications{
			Webhooks: []Webhook{{Name: "w1", URL: "https://discord.com/webhook/secret"}},
		},
		RSS: RSS{
			Feeds: []RSSFeed{{Name: "f1", URL: "https://indexer.com/rss?apikey=xxx"}},
		},
	}
	redacted := cfg.Redacted()

	if redacted.Servers[0].Password != "********" {
		t.Errorf("password not redacted: %q", redacted.Servers[0].Password)
	}
	if redacted.API.APIKey != "********" {
		t.Errorf("api key not redacted: %q", redacted.API.APIKey)
	}
	if redacted.Notifications.Webhooks[0].URL != "********" {
		t.Errorf("webhook URL not redacted: %q", redacted.Notifications.Webhooks[0].URL)
	}
	if redacted.RSS.Feeds[0].URL != "********" {
		t.Errorf("RSS URL not redacted: %q", redacted.RSS.Feeds[0].URL)
	}
	// Original should be unchanged
	if cfg.Servers[0].Password != "secret123" {
		t.Error("original config was modified")
	}
}

func TestMergeRedacted_PreservesSecrets(t *testing.T) {
	original := &Config{
		Servers: []Server{{Name: "s1", Host: "news.com", Port: 563, Password: "real-pass"}},
		API:     API{APIKey: "real-key"},
	}
	edited := &Config{
		Servers: []Server{{Name: "s1", Host: "new-host.com", Port: 563, Password: "********"}},
		API:     API{APIKey: "********"},
	}
	MergeRedacted(edited, original)

	if edited.Servers[0].Password != "real-pass" {
		t.Errorf("password = %q, want real-pass", edited.Servers[0].Password)
	}
	if edited.API.APIKey != "real-key" {
		t.Errorf("api key = %q, want real-key", edited.API.APIKey)
	}
	// Non-redacted fields should keep the edited values
	if edited.Servers[0].Host != "new-host.com" {
		t.Errorf("host = %q, want new-host.com", edited.Servers[0].Host)
	}
}

func TestMergeRedacted_AllowsPasswordChange(t *testing.T) {
	original := &Config{
		Servers: []Server{{Password: "old-pass"}},
		API:     API{APIKey: "old-key"},
	}
	edited := &Config{
		Servers: []Server{{Password: "new-pass"}},
		API:     API{APIKey: "new-key"},
	}
	MergeRedacted(edited, original)

	if edited.Servers[0].Password != "new-pass" {
		t.Errorf("password = %q, want new-pass", edited.Servers[0].Password)
	}
	if edited.API.APIKey != "new-key" {
		t.Errorf("api key = %q, want new-key", edited.API.APIKey)
	}
}

func TestManagerGetAndUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	initial := &Config{
		API:     API{Port: 8080},
		Servers: []Server{{Name: "s1", Host: "news.com", Port: 563}},
	}
	mgr := NewManager(path, initial)

	if mgr.Get().API.Port != 8080 {
		t.Errorf("initial port = %d", mgr.Get().API.Port)
	}
	if mgr.FilePath() != path {
		t.Errorf("path = %q", mgr.FilePath())
	}

	updated := &Config{
		API:     API{Port: 9090},
		Servers: []Server{{Name: "s1", Host: "new.com", Port: 563}},
	}
	if err := mgr.Update(updated); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if mgr.Get().API.Port != 9090 {
		t.Errorf("updated port = %d", mgr.Get().API.Port)
	}
	if mgr.Get().Servers[0].Host != "new.com" {
		t.Errorf("updated host = %q", mgr.Get().Servers[0].Host)
	}
}

func TestManagerUpdate_InvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	mgr := NewManager(path, &Config{API: API{Port: 8080}})

	// Server with empty host should fail validation
	err := mgr.Update(&Config{
		Servers: []Server{{Name: "bad", Port: 563}},
	})
	if err == nil {
		t.Error("expected validation error")
	}
	// Original config should be unchanged
	if mgr.Get().API.Port != 8080 {
		t.Error("config should not have changed on failed update")
	}
}

func TestForwardAuthDefaults(t *testing.T) {
	content := `
servers:
  - name: test
    host: news.com
    port: 563
api:
  forward_auth:
    enabled: true
`
	path := writeTemp(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.API.ForwardAuth.UserHeader != "Remote-User" {
		t.Errorf("UserHeader = %q", cfg.API.ForwardAuth.UserHeader)
	}
	if cfg.API.ForwardAuth.GroupsHeader != "Remote-Groups" {
		t.Errorf("GroupsHeader = %q", cfg.API.ForwardAuth.GroupsHeader)
	}
}

func TestPostProcessDefaults(t *testing.T) {
	content := `
servers:
  - name: test
    host: news.com
    port: 563
`
	path := writeTemp(t, content)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PostProcess.Par2Path != "par2" {
		t.Errorf("Par2Path = %q", cfg.PostProcess.Par2Path)
	}
	if cfg.PostProcess.SevenzPath != "7z" {
		t.Errorf("SevenzPath = %q", cfg.PostProcess.SevenzPath)
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
