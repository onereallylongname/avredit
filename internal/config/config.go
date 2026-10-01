// Package config handles user configuration for avredit.
// Configuration is loaded from ~/.config/avredit/config.json.
package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
)

// Config holds user preferences.
type Config struct {
	Theme     string `json:"theme"`      // Default theme name
	ThemesDir string `json:"themes_dir"` // Custom themes directory (optional)
	LogLevel  string `json:"log_level"`  // Log level: debug, info, warn, error (default: error)

	// RegistryURL is the default Schema Registry base URL (e.g. a Confluent
	// Cloud endpoint). Only the URL is persisted; credentials are never
	// written to disk and must be supplied per-session via --token/--user/
	// --password CLI flags (see cmd/avredit/main.go).
	RegistryURL string `json:"registry_url,omitempty"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Theme:    "dark",
		LogLevel: "error",
	}
}

// Load reads the config from the standard path (~/.config/avredit/config.json).
// Returns default config if the file doesn't exist.
func Load() Config {
	cfg := DefaultConfig()
	path := FilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("config read failed, using defaults", "path", path, "error", err)
		}
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		slog.Warn("config parse failed, using defaults", "path", path, "error", err)
	}
	return cfg
}

// Dir returns the config directory path.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "avredit")
}

// FilePath returns the full path to the config file.
func FilePath() string {
	return filepath.Join(Dir(), "config.json")
}

// ThemesDirFromConfig returns the themes directory, preferring config override.
func (c Config) ThemesDirResolved() string {
	if c.ThemesDir != "" {
		return c.ThemesDir
	}
	return ""
}

// Save writes the config to the standard path (~/.config/avredit/config.json).
func (c Config) Save() error {
	dir := Dir()
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(FilePath(), data, 0644)
}

// SetTheme updates the theme name in the config and persists it.
func SetTheme(name string) {
	cfg := Load()
	cfg.Theme = name
	if err := cfg.Save(); err != nil {
		slog.Error("config save failed", "path", FilePath(), "error", err)
	}
}
