# Buildermark Local Linux Flatpak (Codex)

This folder contains a **tray-only** Linux desktop wrapper for Buildermark Local, packaged as Flatpak.

It launches the Buildermark Local Go server binary in the background and provides a tray menu with:

1. **Status: ...** (running/stopped)
2. **Open Buildermark Local** (opens `http://localhost:7022`)
3. divider
4. **Settings** (window with link to `https://buildermark.dev`)
5. **Quit**

## What's included

- `dev.buildermark.LocalTray.yml` — Flatpak manifest
- `src/main.py` — GTK tray application
- `data/dev.buildermark.LocalTray.desktop` — desktop launcher metadata
- `data/dev.buildermark.LocalTray.appdata.xml` — appstream metadata
- `data/dev.buildermark.LocalTray.svg` — app icon

## Prerequisites

Install Flatpak + Flatpak Builder and add Flathub:

```bash
flatpak --version
flatpak-builder --version
flatpak remote-add --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo
```

Install required SDK/runtime:

```bash
flatpak install -y flathub org.gnome.Platform//47 org.gnome.Sdk//47
```

## Build and run locally

From repository root:

```bash
cd apps/linux_flatpak_codex
flatpak-builder --force-clean build-dir dev.buildermark.LocalTray.yml
flatpak-builder --run build-dir dev.buildermark.LocalTray.yml buildermark-local-tray
```

## Install the Flatpak on your machine

```bash
cd apps/linux_flatpak_codex
flatpak-builder --force-clean build-dir dev.buildermark.LocalTray.yml
flatpak-builder --repo repo --force-clean build-dir dev.buildermark.LocalTray.yml
flatpak build-bundle repo buildermark-local-tray.flatpak dev.buildermark.LocalTray
flatpak install -y ./buildermark-local-tray.flatpak
flatpak run dev.buildermark.LocalTray
```

## Notes

- The manifest builds the Go server from `local/server/cmd/buildermark` and installs it to `/app/bin/buildermark-local`.
- The app stores its SQLite database in `~/.var/app/dev.buildermark.LocalTray/data/buildermark/local.db`.
- If your desktop environment does not show legacy tray icons by default, enable legacy tray support/extension for your shell.
