# smokepod

`smokepod` on npm is a thin wrapper around the native Smokepod binaries published on GitHub releases. The package does not reimplement the CLI in JavaScript; it uses platform-specific optional dependencies to install the correct binary for your OS and architecture.

## Install

```bash
npm install --save-dev smokepod
```

or:

```bash
pnpm add -D smokepod
```

npm automatically installs only the binary matching your platform from the `@peteretelej/smokepod-*` packages.

## Supported platforms

- Linux `x64` and `arm64`
- macOS `x64` and `arm64`
- Windows `x64` and `arm64`

## Override binary with `SMOKEPOD_BINARY`

If you need to use a local binary instead of the platform package:

```bash
SMOKEPOD_BINARY=/absolute/path/to/smokepod npx smokepod run config.yaml
```

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

- [Full documentation on GitHub](https://github.com/peteretelej/smokepod)
- [Troubleshooting](https://github.com/peteretelej/smokepod/blob/main/docs/troubleshooting.md)
- [Configuration reference](https://github.com/peteretelej/smokepod/blob/main/docs/config-reference.md)

## Local development

- `cd npm && npm test`
- `cd npm && npm pack`

The checked-in `0.0.0-dev` version is source-control-only. The release workflow rewrites it from the Git tag before `npm publish`.
