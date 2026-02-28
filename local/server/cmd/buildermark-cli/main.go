package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/cli"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/updater"
)

var version = "dev"

func main() {
	cli.Version = version
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		printUsage()
		return 0

	case "version", "-v", "--version":
		cli.RunVersion(os.Stdout)
		return 0

	case "status":
		return runStatus()

	case "run":
		return runServer(args[1:])

	case "start":
		return runSystemctl("start")

	case "stop":
		return runSystemctl("stop")

	case "restart":
		return runSystemctl("restart")

	case "logs":
		return runLogs(args[1:])

	case "service":
		return runService(args[1:])

	case "open":
		return runOpen()

	case "update":
		return runUpdate(args[1:])

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", args[0])
		printUsage()
		return 1
	}
}

func printUsage() {
	fmt.Print(`Usage: buildermark <command>

Commands:
  run              Start the server (blocking)
  start            Start the server via systemd
  stop             Stop the server via systemd
  restart          Restart the server via systemd
  logs             Follow server logs via journalctl
  status           Show server status
  open             Open Buildermark in the browser
  service install  Install systemd user service
  service uninstall Remove systemd user service
  update check     Check for available updates
  update apply     Download and install an update
  update mode      Set update mode (auto, check, off)
  version          Print version
  help             Show this help
`)
}

func configDir() string {
	dir, err := cli.DefaultConfigDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return ""
	}
	return dir
}

func dbPath() string {
	if env := os.Getenv("BUILDERMARK_LOCAL_DB_PATH"); env != "" {
		return env
	}
	p, err := cli.DefaultDBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return ""
	}
	return p
}

func runStatus() int {
	dir := configDir()
	db := dbPath()
	result := cli.CheckStatus("http://localhost:7022", dir, db)
	cli.PrintStatus(os.Stdout, result)
	return 0
}

func runServer(args []string) int {
	addr := ":7022"
	db := dbPath()

	// Parse optional flags.
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-addr":
			if i+1 < len(args) {
				addr = args[i+1]
				i++
			}
		case "-db":
			if i+1 < len(args) {
				db = args[i+1]
				i++
			}
		}
	}

	if db == "" {
		fmt.Fprintln(os.Stderr, "error: could not determine database path")
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-done
		cancel()
	}()

	if err := cli.RunServer(ctx, cli.RunOptions{DBPath: db, Addr: addr}); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func runSystemctl(action string) int {
	cmd := cli.ExecCommander{}
	if err := cli.RunSystemctl(cmd, action); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func runLogs(args []string) int {
	lines := 100
	for i := 0; i < len(args); i++ {
		if args[i] == "-n" && i+1 < len(args) {
			n, err := strconv.Atoi(args[i+1])
			if err == nil {
				lines = n
			}
			i++
		}
	}

	cmd := cli.ExecCommander{}
	if err := cli.RunLogs(cmd, lines); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func runService(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: buildermark service <install|uninstall>")
		return 1
	}

	cmd := cli.ExecCommander{}

	switch args[0] {
	case "install":
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		db := dbPath()
		if db == "" {
			fmt.Fprintln(os.Stderr, "error: could not determine database path")
			return 1
		}
		if err := cli.ServiceInstall(cmd, cli.UnitParams{ExecStart: exe, DBPath: db}); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Println("Service installed and started")
		return 0

	case "uninstall":
		if err := cli.ServiceUninstall(cmd); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Println("Service uninstalled")
		return 0

	default:
		fmt.Fprintf(os.Stderr, "unknown service command: %s\n", args[0])
		return 1
	}
}

func runOpen() int {
	cmd := cli.ExecCommander{}
	opener := cli.XDGOpener{Cmd: cmd}
	if err := cli.RunOpen(opener, ":7022"); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func runUpdate(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: buildermark update <check|apply|mode>")
		return 1
	}

	u := updater.GetUpdater(version)

	switch args[0] {
	case "check":
		if err := cli.RunUpdateCheck(os.Stdout, u); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		return 0

	case "apply":
		if err := cli.RunUpdateApply(os.Stdout, u); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		return 0

	case "mode":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: buildermark update mode <auto|check|off>")
			return 1
		}
		dir := configDir()
		if dir == "" {
			return 1
		}
		if err := cli.RunUpdateSetMode(dir, args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Printf("Update mode set to %q\n", args[1])
		return 0

	default:
		fmt.Fprintf(os.Stderr, "unknown update command: %s\n", args[0])
		return 1
	}
}
