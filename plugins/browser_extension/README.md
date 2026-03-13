# Buildermark Browser Extension

Shared MV3 browser extension that imports conversations from coding agent web interfaces into a local Buildermark server.

## Prerequisites

- Buildermark server running locally on port `55022`
- Node.js for manifest generation in `build.sh`
- Xcode for Safari packaging
- ImageMagick for manual icon generation

## Build Targets

Generate loadable browser outputs under `browsers/`:

```bash
cd plugins/browser_extension
./build.sh chromium
./build.sh firefox
./build.sh safari
./build.sh all
```

Browser outputs:

- `browsers/chromium`
- `browsers/firefox`
- `browsers/safari`

Safari also generates a wrapper app project under `plugins/browser_extension/safari/BuildermarkSafari` on first build. To build that host app:

```bash
cd plugins/browser_extension
./safari/build.sh
```

## Icons

To regenerate extension icons manually:

```bash
./plugins/browser_extension/generate-icons.sh
```
