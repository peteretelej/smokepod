import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, rm, writeFile, chmod } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import test from 'node:test';
import { spawnSync } from 'node:child_process';

async function withTempPackage(fn) {
  const tempRoot = await mkdtemp(path.join(os.tmpdir(), 'smokepod-npm-run-'));
  const packageRoot = path.join(tempRoot, 'npm');
  const binDir = path.join(packageRoot, 'bin');

  await mkdir(binDir, { recursive: true });
  await writeFile(path.join(packageRoot, 'package.json'), JSON.stringify({ name: 'smokepod', version: '1.2.3', type: 'module' }, null, 2));
  await writeFile(path.join(binDir, 'run.js'), await readFile(new URL('../bin/run.js', import.meta.url)));
  await chmod(path.join(binDir, 'run.js'), 0o755);

  try {
    await fn({ packageRoot, binDir });
  } finally {
    await rm(tempRoot, { recursive: true, force: true });
  }
}

function runLauncher(packageRoot, args = [], env = {}) {
  return spawnSync(process.execPath, [path.join(packageRoot, 'bin', 'run.js'), ...args], {
    cwd: packageRoot,
    encoding: 'utf8',
    env: { ...process.env, ...env }
  });
}

test('fails with a recovery message when the platform package is missing', async () => {
  await withTempPackage(async ({ packageRoot }) => {
    const result = runLauncher(packageRoot, ['--version'], { SMOKEPOD_BINARY: '' });
    assert.equal(result.status, 1);
    assert.match(result.stderr, /not installed/);
    assert.match(result.stderr, /npm install smokepod/);
  });
});

test('spawns the binary from SMOKEPOD_BINARY override', async () => {
  await withTempPackage(async ({ packageRoot }) => {
    const fakeBinary = path.join(packageRoot, 'fake-smokepod');
    await writeFile(
      fakeBinary,
      [
        '#!/usr/bin/env node',
        "process.stdout.write(JSON.stringify(process.argv.slice(2)));",
        'process.exit(23);'
      ].join('\n')
    );
    await chmod(fakeBinary, 0o755);

    const result = runLauncher(packageRoot, ['--json', 'config.yaml'], {
      SMOKEPOD_BINARY: fakeBinary
    });

    assert.equal(result.status, 23);
    assert.equal(result.stdout, '["--json","config.yaml"]');
    assert.equal(result.stderr, '');
  });
});

test('spawns the binary from a platform package in node_modules', async () => {
  await withTempPackage(async ({ packageRoot }) => {
    const platform = process.platform;
    const arch = process.arch;
    const pkgName = `@peteretelej/smokepod-${platform}-${arch}`;
    const pkgDir = path.join(packageRoot, 'node_modules', '@peteretelej', `smokepod-${platform}-${arch}`);
    const pkgBinDir = path.join(pkgDir, 'bin');

    await mkdir(pkgBinDir, { recursive: true });
    await writeFile(path.join(pkgDir, 'package.json'), JSON.stringify({ name: pkgName, version: '1.2.3' }));

    const binName = platform === 'win32' ? 'smokepod.exe' : 'smokepod';
    const binaryPath = path.join(pkgBinDir, binName);
    await writeFile(
      binaryPath,
      [
        '#!/usr/bin/env node',
        "process.stdout.write('platform-pkg-ok');",
        'process.exit(0);'
      ].join('\n')
    );
    await chmod(binaryPath, 0o755);

    const result = runLauncher(packageRoot, [], { SMOKEPOD_BINARY: '' });

    assert.equal(result.status, 0);
    assert.equal(result.stdout, 'platform-pkg-ok');
    assert.equal(result.stderr, '');
  });
});
