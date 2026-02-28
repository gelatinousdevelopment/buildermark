package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

const unitTemplate = `[Unit]
Description=Buildermark Local Server
After=network.target

[Service]
Type=simple
ExecStart={{.ExecStart}} run
Restart=always
RestartSec=3
Environment=BUILDERMARK_LOCAL_DB_PATH={{.DBPath}}

[Install]
WantedBy=default.target
`

// UnitParams holds the parameters for generating a systemd unit file.
type UnitParams struct {
	ExecStart string // absolute path to the buildermark binary
	DBPath    string // absolute path to the database file
}

// GenerateUnitFile returns the systemd unit file content.
func GenerateUnitFile(params UnitParams) (string, error) {
	tmpl, err := template.New("unit").Parse(unitTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing unit template: %w", err)
	}
	var buf []byte
	w := &byteWriter{buf: &buf}
	if err := tmpl.Execute(w, params); err != nil {
		return "", fmt.Errorf("executing unit template: %w", err)
	}
	return string(buf), nil
}

type byteWriter struct {
	buf *[]byte
}

func (w *byteWriter) Write(p []byte) (int, error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

// userUnitDir returns the systemd user unit directory.
func userUnitDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}

// unitFilePath returns the full path to the buildermark systemd unit file.
func unitFilePath() (string, error) {
	dir, err := userUnitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, serviceName+".service"), nil
}

// ServiceInstall writes the systemd unit file, reloads the daemon, and enables the service.
func ServiceInstall(cmd Commander, params UnitParams) error {
	content, err := GenerateUnitFile(params)
	if err != nil {
		return err
	}

	unitDir, err := userUnitDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		return fmt.Errorf("creating unit directory: %w", err)
	}

	unitPath, err := unitFilePath()
	if err != nil {
		return err
	}
	if err := os.WriteFile(unitPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing unit file: %w", err)
	}

	if err := cmd.Run("systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}
	if err := cmd.Run("systemctl", "--user", "enable", "--now", serviceName); err != nil {
		return fmt.Errorf("enable: %w", err)
	}
	return nil
}

// ServiceUninstall stops and disables the service, reloads the daemon, and removes the unit file.
func ServiceUninstall(cmd Commander) error {
	if err := cmd.Run("systemctl", "--user", "disable", "--now", serviceName); err != nil {
		return fmt.Errorf("disable: %w", err)
	}
	if err := cmd.Run("systemctl", "--user", "daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}

	unitPath, err := unitFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing unit file: %w", err)
	}
	return nil
}
