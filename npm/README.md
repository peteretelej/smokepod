# smokepod npm wrapper

This package is a thin wrapper around Smokepod binaries published on GitHub releases.

Installed binaries live under `vendor/` inside the package.

## Local development

- `cd npm && npm test`
- `cd npm && npm pack`

The checked-in `0.0.0-dev` version is source-control-only. The release workflow rewrites it from the Git tag before `npm publish`.
