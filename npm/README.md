# smokepod

`smokepod` on npm is a thin wrapper around the native Smokepod binaries published on GitHub releases. The package does not reimplement the CLI in JavaScript; it installs the matching release binary during `postinstall` and exposes it through npm's normal `bin` linking.

## Install

```bash
npm install --save-dev smokepod
```

or:

```bash
pnpm add -D smokepod
```

Installed binaries live under `vendor/` inside the package.

## Supported platforms

- Linux `x64` and `arm64`
- macOS `x64` and `arm64`
- Windows `x64` and `arm64`

## Override install source with `SMOKEPOD_BINARY`

If you need to install from a trusted local binary instead of downloading from GitHub releases, set `SMOKEPOD_BINARY` during install:

```bash
SMOKEPOD_BINARY=/absolute/path/to/smokepod npm install --save-dev smokepod
```

The installer copies that binary into `vendor/` and keeps the original file untouched.

## Usage

After install, use `smokepod` from package scripts or through your package manager:

```json
{
  "scripts": {
    "smoke": "smokepod run smokepod.yaml"
  }
}
```

## More documentation

- Main project README: `../README.md`
- Troubleshooting guide: `../docs/troubleshooting.md`
- Configuration reference: `../docs/config-reference.md`

## Local development

- `cd npm && npm test`
- `cd npm && npm pack`

The checked-in `0.0.0-dev` version is source-control-only. The release workflow rewrites it from the Git tag before `npm publish`.
