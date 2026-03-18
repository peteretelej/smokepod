#!/usr/bin/env node

import { constants, realpathSync } from 'node:fs';
import { access } from 'node:fs/promises';
import { spawn } from 'node:child_process';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import process from 'node:process';

const PLATFORM_PACKAGES = new Map([
  ['darwin-arm64', '@peteretelej/smokepod-darwin-arm64'],
  ['darwin-x64', '@peteretelej/smokepod-darwin-x64'],
  ['linux-arm64', '@peteretelej/smokepod-linux-arm64'],
  ['linux-x64', '@peteretelej/smokepod-linux-x64'],
  ['win32-arm64', '@peteretelej/smokepod-win32-arm64'],
  ['win32-x64', '@peteretelej/smokepod-win32-x64']
]);

function isDirectExecution(entryUrl, argv1 = process.argv[1]) {
  if (!argv1) {
    return false;
  }

  try {
    return realpathSync(argv1) === realpathSync(fileURLToPath(entryUrl));
  } catch {
    return false;
  }
}

export function resolveBinaryPath(platform = process.platform, arch = process.arch) {
  if (process.env.SMOKEPOD_BINARY) {
    return process.env.SMOKEPOD_BINARY;
  }

  const key = `${platform}-${arch}`;
  const pkg = PLATFORM_PACKAGES.get(key);
  if (!pkg) {
    console.error(
      `Unsupported platform: ${platform} ${arch}\n` +
      `Supported: ${[...PLATFORM_PACKAGES.keys()].join(', ')}\n` +
      'Fallback: go install github.com/peteretelej/smokepod/cmd/smokepod@latest'
    );
    process.exit(1);
  }

  try {
    const require = createRequire(import.meta.url);
    const pkgJson = require.resolve(`${pkg}/package.json`);
    const binName = platform === 'win32' ? 'smokepod.exe' : 'smokepod';
    return path.join(path.dirname(pkgJson), 'bin', binName);
  } catch {
    console.error(
      `Platform package ${pkg} is not installed.\n` +
      'Try reinstalling: npm install smokepod\n' +
      'Or set SMOKEPOD_BINARY=/path/to/smokepod'
    );
    process.exit(1);
  }
}

export async function main() {
  const binaryPath = resolveBinaryPath();

  try {
    await access(binaryPath, constants.X_OK);
  } catch {
    console.error(
      `smokepod binary not found at ${binaryPath}.\n` +
      'Try reinstalling: npm install smokepod\n' +
      'Or set SMOKEPOD_BINARY=/path/to/smokepod'
    );
    process.exit(1);
  }

  await new Promise((resolve, reject) => {
    const child = spawn(binaryPath, process.argv.slice(2), {
      stdio: 'inherit'
    });

    child.on('error', reject);
    child.on('exit', (code, signal) => {
      resolve({ code, signal });
    });
  }).then(({ code, signal }) => {
    process.exit(signal ? 1 : (code ?? 1));
  });
}

if (isDirectExecution(import.meta.url)) {
  try {
    await main();
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
