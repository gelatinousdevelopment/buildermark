# Homebrew distribution (tap files)

This folder contains the Homebrew artifacts for distributing Buildermark **without rewriting existing apps**:

- `Casks/buildermark.rb` — installs the macOS GUI app (`Buildermark.app`) from a signed DMG.
- `Formula/buildermark-linux.rb` — installs the Linux CLI (`buildermark`) from release tarballs.

## Can we distribute both via Homebrew?

Yes:

- **macOS GUI app**: distribute through a Homebrew **cask** (`brew install --cask buildermark`).
- **Linux CLI**: distribute through a Homebrew **formula** (`brew install buildermark-linux`) on Linuxbrew.

This is the standard Homebrew split: casks for macOS app bundles, formulae for CLI tools.

## Recommended release assets

Publish these release assets (for each version tag, e.g. `v1.2.3`):

- `buildermark-macos-arm64.dmg`
- `buildermark-macos-amd64.dmg`
- `buildermark-linux-amd64.tar.gz` (contains `buildermark`)
- `buildermark-linux-arm64.tar.gz` (contains `buildermark`)

You already have build scripts for macOS and Linux:

- macOS DMG: `apps/macos/scripts/build-and-distribute.sh`
- Linux CLI binary: `apps/linux-cli/scripts/build.sh`

Package Linux tarballs like:

```bash
mkdir -p dist/linux-amd64 && cp apps/linux-cli/build/amd64/buildermark dist/linux-amd64/
tar -C dist/linux-amd64 -czf buildermark-linux-amd64.tar.gz buildermark
```

## How to use these files in a tap repo

Create a tap repository (example: `buildermark/homebrew-tap`) and copy:

- `apps/homebrew/Casks/buildermark.rb` -> `Casks/buildermark.rb`
- `apps/homebrew/Formula/buildermark-linux.rb` -> `Formula/buildermark-linux.rb`

Then users install with:

```bash
brew tap buildermark/tap
brew install --cask buildermark       # macOS app
brew install buildermark-linux        # Linux CLI
```

## Updating checksums per release

Use this helper to print SHA256 checksums from release artifacts:

```bash
./apps/homebrew/scripts/print-sha256.sh <path-to-artifact>
```

Paste the values into the cask/formula before publishing.
