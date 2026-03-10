package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const redactedPlaceholder = "********"

// GenerateAPIKey creates a cryptographically random API key.
// Format: "gobin_" prefix + 48 hex chars (24 random bytes = 192 bits of entropy).
func GenerateAPIKey() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "gobin_" + hex.EncodeToString(b)
}

// Config represents the complete GoBin configuration.
type Config struct {
	General       General       `yaml:"general" json:"general"`
	Servers       []Server      `yaml:"servers" json:"servers"`
	Categories    []Category    `yaml:"categories" json:"categories"`
	Downloads     Downloads     `yaml:"downloads" json:"downloads"`
	Schedule      Schedule      `yaml:"schedule" json:"schedule"`
	PostProcess   PostProcess   `yaml:"postprocess" json:"postprocess"`
	API           API           `yaml:"api" json:"api"`
	Notifications Notifications `yaml:"notifications" json:"notifications"`
	RSS           RSS           `yaml:"rss" json:"rss"`
}

type General struct {
	DownloadDir string `yaml:"download_dir" json:"download_dir"`
	CompleteDir string `yaml:"complete_dir" json:"complete_dir"`
	WatchDir    string `yaml:"watch_dir" json:"watch_dir"`
	Permissions string `yaml:"permissions" json:"permissions"`
	LogLevel    string `yaml:"log_level" json:"log_level"`
}

type Server struct {
	Name        string `yaml:"name" json:"name"`
	Host        string `yaml:"host" json:"host"`
	Port        int    `yaml:"port" json:"port"`
	TLS         bool   `yaml:"tls" json:"tls"`
	Username    string `yaml:"username" json:"username"`
	Password    string `yaml:"password" json:"password"`
	Connections int    `yaml:"connections" json:"connections"`
	Priority    int    `yaml:"priority" json:"priority"`
	Retention   int    `yaml:"retention" json:"retention"`
}

type Category struct {
	Name   string `yaml:"name" json:"name"`
	Dir    string `yaml:"dir" json:"dir"`
	Script string `yaml:"script" json:"script"`
}

type Downloads struct {
	MaxRetries     int    `yaml:"max_retries" json:"max_retries"`
	ArticleCacheMB int   `yaml:"article_cache_mb" json:"article_cache_mb"`
	TempDir        string `yaml:"temp_dir" json:"temp_dir"`
	SpeedLimitKbps int   `yaml:"speed_limit_kbps" json:"speed_limit_kbps"`
}

type Schedule struct {
	Enabled bool           `yaml:"enabled" json:"enabled"`
	Rules   []ScheduleRule `yaml:"rules" json:"rules"`
}

type ScheduleRule struct {
	Days           []string `yaml:"days" json:"days"`
	Start          string   `yaml:"start" json:"start"`
	End            string   `yaml:"end" json:"end"`
	SpeedLimitKbps int      `yaml:"speed_limit_kbps" json:"speed_limit_kbps"`
}

type PostProcess struct {
	Par2Enabled        bool   `yaml:"par2_enabled" json:"par2_enabled"`
	Par2Path           string `yaml:"par2_path" json:"par2_path"`
	UnpackEnabled      bool   `yaml:"unpack_enabled" json:"unpack_enabled"`
	SevenzPath         string `yaml:"sevenz_path" json:"sevenz_path"`
	CleanupAfterUnpack bool   `yaml:"cleanup_after_unpack" json:"cleanup_after_unpack"`
	ScriptDir          string `yaml:"script_dir" json:"script_dir"`
}

type API struct {
	Listen      string      `yaml:"listen" json:"listen"`
	Port        int         `yaml:"port" json:"port"`
	APIKey      string      `yaml:"api_key" json:"api_key"`
	BaseURL     string      `yaml:"base_url" json:"base_url"`
	CORSOrigins []string    `yaml:"cors_origins" json:"cors_origins"`
	ForwardAuth ForwardAuth `yaml:"forward_auth" json:"forward_auth"`
}

type ForwardAuth struct {
	Enabled       bool     `yaml:"enabled" json:"enabled"`
	UserHeader    string   `yaml:"user_header" json:"user_header"`
	NameHeader    string   `yaml:"name_header" json:"name_header"`
	EmailHeader   string   `yaml:"email_header" json:"email_header"`
	GroupsHeader  string   `yaml:"groups_header" json:"groups_header"`
	AllowedGroups []string `yaml:"allowed_groups" json:"allowed_groups"`
}

type Notifications struct {
	OnComplete bool      `yaml:"on_complete" json:"on_complete"`
	OnFailure  bool      `yaml:"on_failure" json:"on_failure"`
	Webhooks   []Webhook `yaml:"webhooks" json:"webhooks"`
}

type Webhook struct {
	Name     string `yaml:"name" json:"name"`
	URL      string `yaml:"url" json:"url"`
	Template string `yaml:"template" json:"template"`
}

type RSS struct {
	Enabled         bool      `yaml:"enabled" json:"enabled"`
	IntervalMinutes int       `yaml:"interval_minutes" json:"interval_minutes"`
	Feeds           []RSSFeed `yaml:"feeds" json:"feeds"`
}

type RSSFeed struct {
	Name     string      `yaml:"name" json:"name"`
	URL      string      `yaml:"url" json:"url"`
	Category string      `yaml:"category" json:"category"`
	Filters  []RSSFilter `yaml:"filters" json:"filters"`
}

type RSSFilter struct {
	Include string `yaml:"include" json:"include"`
	Exclude string `yaml:"exclude" json:"exclude"`
}

// Load reads, expands environment variables in, and parses a YAML config file.
// If the file does not exist, a default config is created at the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config: %w", err)
		}
		if writeErr := createDefault(path); writeErr != nil {
			return nil, fmt.Errorf("creating default config: %w", writeErr)
		}
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	expanded := os.ExpandEnv(string(data))

	cfg := &Config{}
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	applyDefaults(cfg)

	// Auto-generate API key if not set
	if cfg.API.APIKey == "" {
		cfg.API.APIKey = GenerateAPIKey()
		slog.Info("generated new API key (saving to config)")
		// Save back so the key persists across restarts
		_ = Save(path, cfg)
	}

	if err := Validate(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// Save writes a config to the given path as YAML using atomic write.
func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Validate checks a config for errors.
func Validate(cfg *Config) error {
	for i, s := range cfg.Servers {
		if s.Host == "" {
			return fmt.Errorf("server %d: host is required", i)
		}
		if s.Port == 0 {
			return fmt.Errorf("server %d: port is required", i)
		}
	}
	return nil
}

// Redacted returns a copy of the config with sensitive fields masked.
func (c *Config) Redacted() *Config {
	// Deep copy via marshal/unmarshal
	data, _ := yaml.Marshal(c)
	copy := &Config{}
	_ = yaml.Unmarshal(data, copy)

	// Redact server passwords
	for i := range copy.Servers {
		if copy.Servers[i].Password != "" {
			copy.Servers[i].Password = redactedPlaceholder
		}
	}
	// Redact API key
	if copy.API.APIKey != "" {
		copy.API.APIKey = redactedPlaceholder
	}
	// Redact webhook URLs (may contain tokens)
	for i := range copy.Notifications.Webhooks {
		if copy.Notifications.Webhooks[i].URL != "" {
			copy.Notifications.Webhooks[i].URL = redactedPlaceholder
		}
	}
	// Redact RSS feed URLs (may contain API keys)
	for i := range copy.RSS.Feeds {
		if copy.RSS.Feeds[i].URL != "" {
			copy.RSS.Feeds[i].URL = redactedPlaceholder
		}
	}
	return copy
}

// MergeRedacted takes an edited config (potentially with redacted placeholders)
// and restores real secret values from the original where placeholders appear.
func MergeRedacted(edited *Config, original *Config) {
	// Restore server passwords
	for i := range edited.Servers {
		if edited.Servers[i].Password == redactedPlaceholder && i < len(original.Servers) {
			edited.Servers[i].Password = original.Servers[i].Password
		}
	}
	// Restore API key
	if edited.API.APIKey == redactedPlaceholder {
		edited.API.APIKey = original.API.APIKey
	}
	// Restore webhook URLs
	for i := range edited.Notifications.Webhooks {
		if edited.Notifications.Webhooks[i].URL == redactedPlaceholder && i < len(original.Notifications.Webhooks) {
			edited.Notifications.Webhooks[i].URL = original.Notifications.Webhooks[i].URL
		}
	}
	// Restore RSS feed URLs
	for i := range edited.RSS.Feeds {
		if edited.RSS.Feeds[i].URL == redactedPlaceholder && i < len(original.RSS.Feeds) {
			edited.RSS.Feeds[i].URL = original.RSS.Feeds[i].URL
		}
	}
}

func applyDefaults(cfg *Config) {
	if cfg.General.DownloadDir == "" {
		cfg.General.DownloadDir = "/downloads/incomplete"
	}
	if cfg.General.CompleteDir == "" {
		cfg.General.CompleteDir = "/downloads/complete"
	}
	if cfg.General.LogLevel == "" {
		cfg.General.LogLevel = "info"
	}
	if cfg.API.Listen == "" {
		cfg.API.Listen = "0.0.0.0"
	}
	if cfg.API.Port == 0 {
		cfg.API.Port = 8080
	}
	if cfg.Downloads.MaxRetries == 0 {
		cfg.Downloads.MaxRetries = 3
	}
	// Post-processing tool paths
	if cfg.PostProcess.Par2Path == "" {
		cfg.PostProcess.Par2Path = "par2"
	}
	if cfg.PostProcess.SevenzPath == "" {
		cfg.PostProcess.SevenzPath = "7z"
	}
	// Forward auth header defaults
	if cfg.API.ForwardAuth.Enabled {
		if cfg.API.ForwardAuth.UserHeader == "" {
			cfg.API.ForwardAuth.UserHeader = "Remote-User"
		}
		if cfg.API.ForwardAuth.NameHeader == "" {
			cfg.API.ForwardAuth.NameHeader = "Remote-Name"
		}
		if cfg.API.ForwardAuth.EmailHeader == "" {
			cfg.API.ForwardAuth.EmailHeader = "Remote-Email"
		}
		if cfg.API.ForwardAuth.GroupsHeader == "" {
			cfg.API.ForwardAuth.GroupsHeader = "Remote-Groups"
		}
	}
	// Clamp server connections to at least 1
	for i := range cfg.Servers {
		if cfg.Servers[i].Connections < 1 {
			cfg.Servers[i].Connections = 1
		}
	}
}

func createDefault(path string) error {
	content := `# GoBin configuration — edit with your Usenet server details.
# See config.example.yaml for all options.
# Environment variables can be used: host: ${USENET_HOST}

general:
  download_dir: /downloads/incomplete
  complete_dir: /downloads/complete
  log_level: info

servers:
  - name: primary
    host: news.example.com
    port: 563
    tls: true
    username: ""
    password: ""
    connections: 10

api:
  listen: 0.0.0.0
  port: 8080
  api_key: ""
`
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(content), 0644)
}
