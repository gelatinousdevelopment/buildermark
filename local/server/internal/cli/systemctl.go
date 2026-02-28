package cli

import (
	"fmt"
	"os"
	"os/exec"
)

const serviceName = "buildermark"

// Commander abstracts command execution for testability.
type Commander interface {
	Run(name string, args ...string) error
	// RunAttached runs a command with stdout/stderr attached to the terminal.
	RunAttached(name string, args ...string) error
}

// ExecCommander runs real commands via os/exec.
type ExecCommander struct{}

func (ExecCommander) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (ExecCommander) RunAttached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunSystemctl runs a systemctl --user command for the buildermark service.
func RunSystemctl(cmd Commander, action string) error {
	switch action {
	case "start", "stop", "restart":
		return cmd.Run("systemctl", "--user", action, serviceName)
	default:
		return fmt.Errorf("unknown systemctl action: %s", action)
	}
}

// RunLogs runs journalctl to follow buildermark service logs.
func RunLogs(cmd Commander, lines int) error {
	return cmd.RunAttached("journalctl", "--user", "--lines", fmt.Sprintf("%d", lines), "-u", serviceName, "-f")
}
