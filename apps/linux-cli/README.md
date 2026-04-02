# Buildermark Linux CLI

A single `buildermark` binary that runs, manages (via systemd), and updates the Buildermark server on Linux. Supports x86_64 (amd64) and aarch64 (arm64).

## Prerequisites

- **GCC** -- required by go-sqlite3 (CGO)
- **Go** 1.21+
- **Node.js** 18+ and npm

For cross-compilation, install the appropriate cross-compiler:

```bash
# Debian/Ubuntu
sudo apt install build-essential                # native
sudo apt install gcc-aarch64-linux-gnu          # cross-compile to arm64
sudo apt install gcc-x86-64-linux-gnu           # cross-compile to amd64

# Fedora
sudo dnf install gcc                            # native
sudo dnf install gcc-aarch64-linux-gnu          # cross-compile to arm64
sudo dnf install gcc-x86_64-linux-gnu           # cross-compile to amd64
```

## Build

From the repository root:

```bash
./scripts/build-linux.sh                          # host architecture only
./scripts/build-linux.sh --arch all               # both amd64 and arm64
./scripts/build-linux.sh --arch amd64             # x86_64 only
./scripts/build-linux.sh --arch arm64             # aarch64 only
./scripts/build-linux.sh --arch all --version 1.0.0
./scripts/build-linux-vm.sh                       # build in Debian VM from macOS
./scripts/build-linux-vm.sh --arch all --version 1.0.0
```

Binaries are written to `apps/linux-cli/build/<arch>/buildermark`.

On macOS, `scripts/build-linux-vm.sh` starts the `Debian Desktop` UTM VM, waits
for `ssh debianvm`, updates the existing checkout at
`/home/debian/github/buildermark`, overlays only local uncommitted files from
the macOS checkout, runs the Linux build in Debian, copies the binaries back to
`apps/linux-cli/build/<arch>/buildermark`, and leaves the VM running. If no
`--arch` is passed, it builds for the Debian VM's native architecture.

## Install

```bash
curl -fsSL https://github.com/buildermark/buildermark/releases/latest/download/buildermark-install.sh | bash
```

The installer:

- detects `amd64` vs `arm64`
- downloads the matching GitHub release tarball
- installs `buildermark` to `~/.local/bin/buildermark` by default
- prints PATH help and the next commands to run

After install:

```bash
buildermark service install
buildermark open
```

To install somewhere else:

```bash
curl -fsSL https://github.com/buildermark/buildermark/releases/latest/download/buildermark-install.sh | \
  env BUILDERMARK_INSTALL_DIR=/usr/local/bin bash
```

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
