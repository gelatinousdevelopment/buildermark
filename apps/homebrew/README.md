# Homebrew distribution (single-command install)

This folder provides Homebrew files so users can install Buildermark with one command:

```bash
brew tap buildermark/tap
brew install buildermark
```

## Platform behavior

`Formula/buildermark.rb` installs the correct artifact automatically:

- **macOS**: installs `Buildermark.app` from architecture-specific DMG assets.
- **Linux**: installs the `buildermark` CLI from architecture-specific tarballs.

This keeps distribution simple for users while reusing the existing macOS app and Linux CLI builds (no app rewrite).

## Optional cask

`Casks/buildermark-app.rb` is kept as an optional explicit GUI cask for teams that prefer `brew install --cask buildermark-app` on macOS.

## Release assets expected

For each release tag (for example `v1.2.3`), publish:

- `buildermark-macos-arm64.dmg`
- `buildermark-macos-amd64.dmg`
- `buildermark-linux-amd64.tar.gz` (contains `buildermark`)
- `buildermark-linux-arm64.tar.gz` (contains `buildermark`)

## Tap layout

In your tap repository (for example `buildermark/homebrew-tap`), copy:

- `apps/homebrew/Formula/buildermark.rb` -> `Formula/buildermark.rb`
- `apps/homebrew/Casks/buildermark-app.rb` -> `Casks/buildermark-app.rb` (optional)

## Updating checksums

Use:

```bash
./apps/homebrew/scripts/print-sha256.sh <artifact-path> [<artifact-path> ...]
```

Then paste SHA256 values into the formula/cask before publishing.
