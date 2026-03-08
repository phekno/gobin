package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the complete GoBin configuration.
type Config struct {
	General       General       `yaml:"general"`
	Servers       []Server      `yaml:"servers"`
	Categories    []Category    `yaml:"categories"`
	Downloads     Downloads     `yaml:"downloads"`
	Schedule      Schedule      `yaml:"schedule"`
	PostProcess   PostProcess   `yaml:"postprocess"`
	API           API           `yaml:"api"`
	Notifications Notifications `yaml:"notifications"`
	RSS           RSS           `yaml:"rss"`
}

type General struct {
	DownloadDir string `yaml:"download_dir"`
	CompleteDir string `yaml:"complete_dir"`
	WatchDir    string `yaml:"watch_dir"`
	Permissions string `yaml:"permissions"`
	LogLevel    string `yaml:"log_level"`
}

type Server struct {
	Name        string `yaml:"name"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	TLS         bool   `yaml:"tls"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	Connections int    `yaml:"connections"`
	Priority    int    `yaml:"priority"`
	Retention   int    `yaml:"retention"`
}

type Category struct {
	Name   string `yaml:"name"`
	Dir    string `yaml:"dir"`
	Script string `yaml:"script"`
}

type Downloads struct {
	MaxRetries     int    `yaml:"max_retries"`
	ArticleCacheMB int   `yaml:"article_cache_mb"`
	TempDir        string `yaml:"temp_dir"`
	SpeedLimitKbps int   `yaml:"speed_limit_kbps"`
}

type Schedule struct {
	Enabled bool           `yaml:"enabled"`
	Rules   []ScheduleRule `yaml:"rules"`
}

type ScheduleRule struct {
	Days           []string `yaml:"days"`
	Start          string   `yaml:"start"`
	End            string   `yaml:"end"`
	SpeedLimitKbps int      `yaml:"speed_limit_kbps"`
}

type PostProcess struct {
	Par2Enabled        bool   `yaml:"par2_enabled"`
	Par2Path           string `yaml:"par2_path"`
	UnpackEnabled      bool   `yaml:"unpack_enabled"`
	UnrarPath          string `yaml:"unrar_path"`
	SevenzPath         string `yaml:"sevenz_path"`
	CleanupAfterUnpack bool   `yaml:"cleanup_after_unpack"`
	ScriptDir          string `yaml:"script_dir"`
}

type API struct {
	Listen      string   `yaml:"listen"`
	Port        int      `yaml:"port"`
	APIKey      string   `yaml:"api_key"`
	BaseURL     string   `yaml:"base_url"`
	CORSOrigins []string `yaml:"cors_origins"`
}

type Notifications struct {
	OnComplete bool      `yaml:"on_complete"`
	OnFailure  bool      `yaml:"on_failure"`
	Webhooks   []Webhook `yaml:"webhooks"`
}

type Webhook struct {
	Name     string `yaml:"name"`
	URL      string `yaml:"url"`
	Template string `yaml:"template"`
}

type RSS struct {
	Enabled         bool      `yaml:"enabled"`
	IntervalMinutes int       `yaml:"interval_minutes"`
	Feeds           []RSSFeed `yaml:"feeds"`
}

type RSSFeed struct {
	Name     string      `yaml:"name"`
	URL      string      `yaml:"url"`
	Category string      `yaml:"category"`
	Filters  []RSSFilter `yaml:"filters"`
}

type RSSFilter struct {
	Include string `yaml:"include"`
	Exclude string `yaml:"exclude"`
}

// Load reads, expands environment variables in, and parses a YAML config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	// Expand ${ENV_VAR} references
	expanded := os.ExpandEnv(string(data))

	cfg := &Config{}
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
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

	// Clamp server connections to at least 1
	for i := range cfg.Servers {
		if cfg.Servers[i].Connections < 1 {
			cfg.Servers[i].Connections = 1
		}
	}
}

func validate(cfg *Config) error {
	if len(cfg.Servers) == 0 {
		return fmt.Errorf("at least one server must be configured")
	}
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
