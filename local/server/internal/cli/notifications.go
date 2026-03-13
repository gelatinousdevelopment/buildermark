package cli

import (
	"fmt"
	"io"
)

// RunNotificationsGet prints the current notifications setting ("on" or "off").
func RunNotificationsGet(w io.Writer, configDir string) error {
	cfg, err := LoadConfig(configDir)
	if err != nil {
		return err
	}
	if cfg.NotificationsEnabled == nil || *cfg.NotificationsEnabled {
		fmt.Fprintln(w, "on")
	} else {
		fmt.Fprintln(w, "off")
	}
	return nil
}

// RunNotificationsSet sets the notifications enabled flag and saves the config.
func RunNotificationsSet(configDir string, enabled bool) error {
	cfg, err := LoadConfig(configDir)
	if err != nil {
		return err
	}
	cfg.NotificationsEnabled = &enabled
	return SaveConfig(configDir, cfg)
}
