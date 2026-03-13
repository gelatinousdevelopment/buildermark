package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds persistent CLI settings.
type Config struct {
	UpdateMode           string   `json:"updateMode"`                     // "auto", "check", or "off"
	ExtraAgentHomes      []string `json:"extraAgentHomes,omitempty"`
	NotificationsEnabled *bool    `json:"notificationsEnabled,omitempty"`
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() Config {
	return Config{
		UpdateMode: "check",
	}
}

// DefaultConfigDir returns the default configuration directory (~/.buildermark).
func DefaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".buildermark"), nil
}

// DefaultDBPath returns the default database path (~/.buildermark/local.db).
func DefaultDBPath() (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "local.db"), nil
}

// ConfigPath returns the path to the config file within the given directory.
func ConfigPath(dir string) string {
	return filepath.Join(dir, "config.json")
}

// LoadConfig reads the config from the given directory.
// If the file doesn't exist, default values are returned.
func LoadConfig(dir string) (Config, error) {
	path := ConfigPath(dir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return Config{}, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config %s: %w", path, err)
	}
	return cfg, nil
}

// SaveConfig writes the config to the given directory, creating it if needed.
func SaveConfig(dir string, cfg Config) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	data = append(data, '\n')

	path := ConfigPath(dir)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
