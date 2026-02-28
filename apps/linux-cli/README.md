# Buildermark Linux CLI

A single `buildermark` binary that runs, manages (via systemd), and updates the Buildermark server on Linux.

## Prerequisites

- **GCC** (or another C compiler) -- required by go-sqlite3
- **Go** 1.21+
- **Node.js** 18+ and npm

On Debian/Ubuntu:

```bash
sudo apt install build-essential
```

On Fedora:

```bash
sudo dnf install gcc
```

## Build

From the repository root:

```bash
./scripts/build-linux.sh [VERSION]
```

The binary is written to `apps/linux-cli/buildermark`.

## Install

```bash
cp apps/linux-cli/buildermark ~/.local/bin/buildermark
buildermark service install
```

This installs a systemd user service that starts automatically on login.

## Usage

```
buildermark run              # start server (blocking, foreground)
buildermark start            # start via systemd
buildermark stop             # stop via systemd
buildermark restart          # restart via systemd
buildermark logs             # follow journalctl logs
buildermark status           # show server status
buildermark open             # open in browser (xdg-open)
buildermark service install  # install systemd user service
buildermark service uninstall
buildermark update check     # check for updates
buildermark update apply     # download and install update
buildermark update mode <auto|check|off>
buildermark version
buildermark help
```

## Systemd

A reference unit file is at `systemd/buildermark.service`. The `buildermark service install` command generates and installs one automatically.

Data is stored in `~/.buildermark/` (config and database).
