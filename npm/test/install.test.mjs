import assert from 'node:assert/strict';
import { access, mkdtemp, mkdir, readFile, rename, rm, writeFile } from 'node:fs/promises';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import test from 'node:test';
import { pathToFileURL } from 'node:url';
import { spawnSync } from 'node:child_process';

import {
  getArtifactName,
  getReleasePlatform,
  getReleaseUrls,
  getVendorPath,
  main,
  replaceFileAtomically,
  requestBuffer
} from '../install.mjs';

const FIXTURE_BINARY = Buffer.from('fixture-smokepod-binary\n', 'utf8');
const FIXTURE_CHECKSUMS = await readFile(new URL('./fixtures/checksums.txt', import.meta.url), 'utf8');

async function withTempPackage(fn, version = '1.2.3') {
  const tempRoot = await mkdtemp(path.join(os.tmpdir(), 'smokepod-npm-install-'));
  const packageRoot = path.join(tempRoot, 'npm');

  await mkdir(path.join(packageRoot, 'vendor'), { recursive: true });
  await writeFile(
    path.join(packageRoot, 'package.json'),
    JSON.stringify({ name: 'smokepod', version }, null, 2)
  );
  await writeFile(path.join(packageRoot, 'install.mjs'), await readFile(new URL('../install.mjs', import.meta.url)));

  try {
    await fn({ tempRoot, packageRoot });
  } finally {
    await rm(tempRoot, { recursive: true, force: true });
  }
}

function makeRequestImpl({ redirects = false } = {}) {
  const calls = [];
  const redirectBinary = 'https://downloads.example/smokepod';
  const redirectChecksums = 'https://downloads.example/checksums.txt';

  return {
    calls,
    requestImpl: async (url) => {
      calls.push(url);

      if (redirects && url.includes('/releases/download/')) {
        return {
          statusCode: 302,
          headers: {
            location: url.endsWith('checksums.txt') ? redirectChecksums : redirectBinary
          },
          body: Buffer.alloc(0)
        };
      }

      if (url.endsWith('checksums.txt') || url === redirectChecksums) {
        return {
          statusCode: 200,
          headers: {},
          body: Buffer.from(FIXTURE_CHECKSUMS, 'utf8')
        };
      }

      return {
        statusCode: 200,
        headers: {},
        body: FIXTURE_BINARY
      };
    }
  };
}

test('maps all supported platform and architecture pairs', () => {
  const cases = [
    ['linux', 'x64', { os: 'linux', arch: 'amd64' }],
    ['linux', 'arm64', { os: 'linux', arch: 'arm64' }],
    ['darwin', 'x64', { os: 'darwin', arch: 'amd64' }],
    ['darwin', 'arm64', { os: 'darwin', arch: 'arm64' }],
    ['win32', 'x64', { os: 'windows', arch: 'amd64' }],
    ['win32', 'arm64', { os: 'windows', arch: 'arm64' }]
  ];

  for (const [platform, arch, expected] of cases) {
    assert.deepEqual(getReleasePlatform(platform, arch), expected);
  }
});

test('builds all supported artifact filenames', () => {
  const targets = [
    [{ os: 'linux', arch: 'amd64' }, 'smokepod_linux_amd64'],
    [{ os: 'linux', arch: 'arm64' }, 'smokepod_linux_arm64'],
    [{ os: 'darwin', arch: 'amd64' }, 'smokepod_darwin_amd64'],
    [{ os: 'darwin', arch: 'arm64' }, 'smokepod_darwin_arm64'],
    [{ os: 'windows', arch: 'amd64' }, 'smokepod_windows_amd64.exe'],
    [{ os: 'windows', arch: 'arm64' }, 'smokepod_windows_arm64.exe']
  ];

  for (const [target, expected] of targets) {
    assert.equal(getArtifactName(target), expected);
  }
});

test('builds release URLs from package version', () => {
  assert.deepEqual(getReleaseUrls('1.2.3', { os: 'darwin', arch: 'arm64' }), {
    binaryUrl: 'https://github.com/peteretelej/smokepod/releases/download/v1.2.3/smokepod_darwin_arm64',
    checksumsUrl: 'https://github.com/peteretelej/smokepod/releases/download/v1.2.3/checksums.txt'
  });
});

test('selects checksum entries for each supported artifact name', async () => {
  const cases = [
    ['linux', 'x64'],
    ['linux', 'arm64'],
    ['darwin', 'x64'],
    ['darwin', 'arm64'],
    ['win32', 'x64'],
    ['win32', 'arm64']
  ];

  for (const [platform, arch] of cases) {
    await withTempPackage(async ({ packageRoot }) => {
      const { requestImpl } = makeRequestImpl();
      const installedPath = await main({ packageRoot, platform, arch, requestImpl, env: {} });
      assert.equal(installedPath, path.join(packageRoot, getVendorPath(platform)));
      assert.deepEqual(await readFile(installedPath), FIXTURE_BINARY);
    });
  }
});

test('follows redirect responses while downloading release assets', async () => {
  await withTempPackage(async ({ packageRoot }) => {
    const { calls, requestImpl } = makeRequestImpl({ redirects: true });
    await main({ packageRoot, platform: 'linux', arch: 'x64', requestImpl, env: {} });
    assert.equal(calls.length, 4);
    assert.ok(calls.some((url) => url.startsWith('https://downloads.example/')));
  });
});

test('formats unsupported platform failures through install errors', async () => {
  await withTempPackage(async ({ packageRoot }) => {
    await assert.rejects(
      () => main({ packageRoot, platform: 'freebsd', arch: 'x64', env: {} }),
      /Reason: unsupported platform: freebsd/
    );
  });
});

test('formats unsupported architecture failures through install errors', async () => {
  await withTempPackage(async ({ packageRoot }) => {
    await assert.rejects(
      () => main({ packageRoot, platform: 'linux', arch: 'ia32', env: {} }),
      /Reason: unsupported architecture: ia32/
    );
  });
});

test('rejects the development version without SMOKEPOD_BINARY', async () => {
  await withTempPackage(async ({ packageRoot }) => {
    await assert.rejects(
      () => main({ packageRoot, platform: 'linux', arch: 'x64', env: {} }),
      /Reason: source-control development version cannot be installed from GitHub releases without SMOKEPOD_BINARY/
    );
  }, '0.0.0-dev');
});

test('rejects an unreadable SMOKEPOD_BINARY path', async () => {
  await withTempPackage(async ({ packageRoot }) => {
    await assert.rejects(
      () => main({ packageRoot, env: { SMOKEPOD_BINARY: path.join(packageRoot, 'missing-smokepod') } }),
      /Reason: ENOENT/
    );
  });
});

test('rejects checksum mismatches', async () => {
  await withTempPackage(async ({ packageRoot }) => {
    await assert.rejects(
      () => main({
        packageRoot,
        platform: 'linux',
        arch: 'x64',
        env: {},
        requestImpl: async (url) => ({
          statusCode: 200,
          headers: {},
          body: Buffer.from(url.endsWith('checksums.txt') ? FIXTURE_CHECKSUMS : 'wrong-binary', 'utf8')
        })
      }),
      /Reason: checksum mismatch/
    );
  });
});

test('surfaces request timeouts through formatted install errors', async () => {
  await withTempPackage(async ({ packageRoot }) => {
    await assert.rejects(
      () => main({
        packageRoot,
        platform: 'linux',
        arch: 'x64',
        env: {},
        requestImpl: async () => {
          throw new Error('request timed out after 50ms fetching https://example.test/smokepod');
        }
      }),
      /Reason: request timed out after 50ms fetching https:\/\/example.test\/smokepod/
    );
  });
});

test('times out stalled network downloads with install-friendly errors', async () => {
  const server = http.createServer(() => {
    // Intentionally never responds so the client timeout fires.
  });

  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  const url = `http://127.0.0.1:${address.port}/stall`;

  try {
    await assert.rejects(
      () => requestBuffer(url, { timeoutMs: 50 }),
      /request timed out after 50ms fetching/
    );
  } finally {
    server.closeAllConnections();
    await new Promise((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())));
  }
});

test('preserves the backup when replacement rollback also fails', async () => {
  await withTempPackage(async ({ packageRoot }) => {
    const vendorDir = path.join(packageRoot, 'vendor');
    const paths = {
      tempPath: path.join(vendorDir, '.smokepod.tmp'),
      finalPath: path.join(vendorDir, 'smokepod'),
      backupPath: path.join(vendorDir, '.smokepod.backup')
    };

    await writeFile(paths.tempPath, 'new binary');
    await writeFile(paths.finalPath, 'old binary');

    let renameCalls = 0;
    await assert.rejects(
      () => replaceFileAtomically(paths, {
        rm,
        rename: async (from, to) => {
          renameCalls += 1;

          if (renameCalls === 1) {
            const error = new Error('target busy');
            error.code = 'EPERM';
            throw error;
          }

          if (renameCalls === 3) {
            throw new Error('write failed');
          }

          if (renameCalls === 4) {
            throw new Error('restore failed');
          }

          return rename(from, to);
        }
      }),
      /write failed; rollback failed: restore failed; preserved backup/
    );

    await assert.rejects(() => access(paths.finalPath), /ENOENT/);
    assert.equal(await readFile(paths.backupPath, 'utf8'), 'old binary');
    assert.equal(await readFile(paths.tempPath, 'utf8'), 'new binary');
  });
});

test('imports stay side-effect-free while direct execution runs install flow', async () => {
  await withTempPackage(async ({ packageRoot }) => {
    const imported = await import(`${pathToFileURL(path.join(packageRoot, 'install.mjs')).href}?import-check`);
    assert.equal(typeof imported.main, 'function');
    await assert.rejects(() => access(path.join(packageRoot, getVendorPath('linux'))), /ENOENT/);

    const sourceBinary = path.join(packageRoot, 'source-smokepod');
    await writeFile(sourceBinary, '#!/bin/sh\nexit 0\n');
    const execResult = spawnSync(process.execPath, ['install.mjs'], {
      cwd: packageRoot,
      env: { ...process.env, SMOKEPOD_BINARY: sourceBinary },
      encoding: 'utf8'
    });

    assert.equal(execResult.status, 0, execResult.stderr);
    assert.deepEqual(await readFile(path.join(packageRoot, getVendorPath(process.platform))), Buffer.from('#!/bin/sh\nexit 0\n'));
  });
});
