package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfigDir(t *testing.T) {
	dir, err := DefaultConfigDir()
	if err != nil {
		t.Fatalf("DefaultConfigDir() error: %v", err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".buildermark")
	if dir != want {
		t.Errorf("DefaultConfigDir() = %q, want %q", dir, want)
	}
}

func TestDefaultDBPath(t *testing.T) {
	p, err := DefaultDBPath()
	if err != nil {
		t.Fatalf("DefaultDBPath() error: %v", err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".buildermark", "local.db")
	if p != want {
		t.Errorf("DefaultDBPath() = %q, want %q", p, want)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if cfg.UpdateMode != "check" {
		t.Errorf("UpdateMode = %q, want %q", cfg.UpdateMode, "check")
	}
}

func TestLoadConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Config{UpdateMode: "auto"}

	if err := SaveConfig(dir, want); err != nil {
		t.Fatalf("SaveConfig() error: %v", err)
	}

	got, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if got != want {
		t.Errorf("LoadConfig() = %+v, want %+v", got, want)
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(dir)
	if err == nil {
		t.Error("LoadConfig() expected error for invalid JSON, got nil")
	}
}

func TestSaveConfig_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	cfg := Config{UpdateMode: "off"}
	if err := SaveConfig(dir, cfg); err != nil {
		t.Fatalf("SaveConfig() error: %v", err)
	}

	got, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	if got != cfg {
		t.Errorf("round-trip got %+v, want %+v", got, cfg)
	}
}
