import { constants, realpathSync } from 'node:fs';
import { access, chmod, copyFile, mkdir, readFile, rename, rm, stat, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import http from 'node:http';
import https from 'node:https';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const PACKAGE_ROOT = path.dirname(fileURLToPath(import.meta.url));
const RELEASE_REPOSITORY = 'https://github.com/peteretelej/smokepod/releases/download';
const REDIRECT_STATUS_CODES = new Set([301, 302, 307, 308]);
const MAX_REDIRECTS = 5;
export const REQUEST_TIMEOUT_MS = 30_000;

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

export const CHECKSUM_FILE = 'checksums.txt';

export function getReleasePlatform(platform = process.platform, arch = process.arch) {
  const os = new Map([
    ['linux', 'linux'],
    ['darwin', 'darwin'],
    ['win32', 'windows']
  ]).get(platform);
  if (!os) {
    throw new Error(`unsupported platform: ${platform}`);
  }

  const normalizedArch = new Map([
    ['x64', 'amd64'],
    ['arm64', 'arm64']
  ]).get(arch);
  if (!normalizedArch) {
    throw new Error(`unsupported architecture: ${arch}`);
  }

  return { os, arch: normalizedArch };
}

export function getArtifactName(target) {
  if (!target || !target.os || !target.arch) {
    throw new Error('target must include os and arch');
  }

  const validOs = new Set(['linux', 'darwin', 'windows']);
  const validArch = new Set(['amd64', 'arm64']);
  if (!validOs.has(target.os)) {
    throw new Error(`unsupported release os: ${target.os}`);
  }
  if (!validArch.has(target.arch)) {
    throw new Error(`unsupported release architecture: ${target.arch}`);
  }

  return target.os === 'windows'
    ? `smokepod_${target.os}_${target.arch}.exe`
    : `smokepod_${target.os}_${target.arch}`;
}

export function getReleaseTag(version) {
  return `v${version}`;
}

export function getReleaseUrls(version, target) {
  const artifactName = getArtifactName(target);
  const tag = getReleaseTag(version);

  return {
    binaryUrl: `${RELEASE_REPOSITORY}/${tag}/${artifactName}`,
    checksumsUrl: `${RELEASE_REPOSITORY}/${tag}/${CHECKSUM_FILE}`
  };
}

export function getVendorBinaryName(platform = process.platform) {
  return platform === 'win32' ? 'smokepod.exe' : 'smokepod';
}

export function getVendorPath(platform = process.platform) {
  return path.join('vendor', getVendorBinaryName(platform));
}

// Example install error shape:
// smokepod install failed
// Platform: linux
// Architecture: amd64
// Tried: https://github.com/peteretelej/smokepod/releases/download/v1.2.3/smokepod_linux_amd64
// Reason: checksum mismatch
// Retry: SMOKEPOD_BINARY=/absolute/path/to/smokepod npm install
// Fallback: go install github.com/peteretelej/smokepod/cmd/smokepod@v1.2.3
export function formatInstallError({ detectedPlatform, detectedArch, attemptedSource, reason, version }) {
  return [
    'smokepod install failed',
    `Platform: ${detectedPlatform || 'unknown'}`,
    `Architecture: ${detectedArch || 'unknown'}`,
    `Tried: ${attemptedSource || 'unknown'}`,
    `Reason: ${reason || 'unknown error'}`,
    'Retry: SMOKEPOD_BINARY=/absolute/path/to/smokepod npm install',
    `Fallback: go install github.com/peteretelej/smokepod/cmd/smokepod@${getReleaseTag(version || '0.0.0-dev')}`
  ].join('\n');
}

async function readPackageVersion(packageRoot) {
  const packageJsonPath = path.join(packageRoot, 'package.json');
  const packageJson = JSON.parse(await readFile(packageJsonPath, 'utf8'));
  return packageJson.version;
}

function isDevVersion(version) {
  return version === '0.0.0-dev' || /^0\.0\.0-/.test(version) || /(^|[-.])dev($|[-.])/i.test(version);
}

function getInstallPaths(packageRoot, platform) {
  const binaryName = getVendorBinaryName(platform);
  const vendorDir = path.join(packageRoot, 'vendor');

  return {
    binaryName,
    vendorDir,
    finalPath: path.join(packageRoot, getVendorPath(platform)),
    tempPath: path.join(vendorDir, `.${binaryName}.tmp`),
    downloadPath: path.join(vendorDir, `.${binaryName}.download`),
    backupPath: path.join(vendorDir, `.${binaryName}.backup`)
  };
}

function getAttemptedReleaseSource(version, platform, arch) {
  try {
    const target = getReleasePlatform(platform, arch);
    return getReleaseUrls(version, target).binaryUrl;
  } catch {
    return 'local platform detection';
  }
}

async function ensureVendorDirectory(vendorDir) {
  await mkdir(vendorDir, { recursive: true });
}

async function cleanupInstallTemps(paths, { removeBackup = false } = {}) {
  const cleanupTasks = [
    rm(paths.tempPath, { force: true }),
    rm(paths.downloadPath, { force: true })
  ];

  if (removeBackup) {
    cleanupTasks.push(rm(paths.backupPath, { force: true }));
  }

  await Promise.allSettled(cleanupTasks);
}

async function validateReadableFile(filePath) {
  await access(filePath, constants.R_OK);
  const sourceStat = await stat(filePath);
  if (!sourceStat.isFile()) {
    throw new Error(`path is not a file: ${filePath}`);
  }
}

export async function replaceFileAtomically(paths, fileOps = { rename, rm }) {
  try {
    await fileOps.rename(paths.tempPath, paths.finalPath);
    return;
  } catch (error) {
    if (!error || (error.code !== 'EEXIST' && error.code !== 'EPERM')) {
      throw error;
    }
  }

  await fileOps.rm(paths.backupPath, { force: true });

  let movedExisting = false;
  try {
    await fileOps.rename(paths.finalPath, paths.backupPath);
    movedExisting = true;
  } catch (error) {
    if (!error || error.code !== 'ENOENT') {
      throw error;
    }
  }

  try {
    await fileOps.rename(paths.tempPath, paths.finalPath);
  } catch (error) {
    if (movedExisting) {
      try {
        await fileOps.rename(paths.backupPath, paths.finalPath);
      } catch (rollbackError) {
        throw new Error(
          `${error.message}; rollback failed: ${rollbackError.message}; preserved backup at ${paths.backupPath}`
        );
      }
    }

    throw error;
  }

  if (movedExisting) {
    await fileOps.rm(paths.backupPath, { force: true });
  }
}

async function finalizeInstall(paths, platform) {
  if (platform !== 'win32') {
    await chmod(paths.tempPath, 0o755);
  }

  await replaceFileAtomically(paths);
  await access(paths.finalPath, constants.F_OK);
}

export function requestBuffer(url, { timeoutMs = REQUEST_TIMEOUT_MS } = {}) {
  const transport = url.startsWith('https:') ? https : http;

  return new Promise((resolve, reject) => {
    let settled = false;
    const request = transport.get(url, (response) => {
      const chunks = [];

      response.on('error', (error) => {
        if (settled) {
          return;
        }

        settled = true;
        reject(error);
      });

      response.on('data', (chunk) => chunks.push(chunk));
      response.on('end', () => {
        if (settled) {
          return;
        }

        settled = true;
        resolve({
          statusCode: response.statusCode ?? 0,
          headers: response.headers,
          body: Buffer.concat(chunks)
        });
      });
    });

    request.setTimeout(timeoutMs, () => {
      request.destroy(new Error(`request timed out after ${timeoutMs}ms fetching ${url}`));
    });

    request.on('error', (error) => {
      if (settled) {
        return;
      }

      settled = true;
      reject(error);
    });
  });
}

async function fetchWithRedirects(url, requestImpl, redirectCount = 0) {
  const response = await requestImpl(url);

  if (REDIRECT_STATUS_CODES.has(response.statusCode)) {
    if (redirectCount >= MAX_REDIRECTS) {
      throw new Error(`too many redirects fetching ${url}`);
    }

    const location = response.headers.location;
    if (!location) {
      throw new Error(`redirect missing location for ${url}`);
    }

    return fetchWithRedirects(new URL(location, url).toString(), requestImpl, redirectCount + 1);
  }

  if (response.statusCode < 200 || response.statusCode >= 300) {
    throw new Error(`unexpected status ${response.statusCode} fetching ${url}`);
  }

  return response.body;
}

function parseChecksums(contents) {
  const entries = new Map();

  for (const line of contents.split(/\r?\n/)) {
    const match = line.match(/^([a-fA-F0-9]{64})\s+(.+)$/);
    if (!match) {
      continue;
    }

    entries.set(match[2].trim(), match[1].toLowerCase());
  }

  return entries;
}

function sha256(buffer) {
  return createHash('sha256').update(buffer).digest('hex');
}

async function downloadVerifiedBinary(version, target, requestImpl) {
  const { binaryUrl, checksumsUrl } = getReleaseUrls(version, target);
  const artifactName = getArtifactName(target);
  const [binaryBuffer, checksumsBuffer] = await Promise.all([
    fetchWithRedirects(binaryUrl, requestImpl),
    fetchWithRedirects(checksumsUrl, requestImpl)
  ]);

  const checksums = parseChecksums(checksumsBuffer.toString('utf8'));
  const expectedChecksum = checksums.get(artifactName);
  if (!expectedChecksum) {
    throw new Error(`missing checksum entry for ${artifactName}`);
  }

  const actualChecksum = sha256(binaryBuffer);
  if (actualChecksum !== expectedChecksum) {
    throw new Error('checksum mismatch');
  }

  return { binaryBuffer, binaryUrl };
}

async function installBinary({ packageRoot, env, platform, arch, requestImpl }) {
  const version = await readPackageVersion(packageRoot);
  const paths = getInstallPaths(packageRoot, platform);
  let attemptedSource = path.join(packageRoot, getVendorPath(platform));
  let installSucceeded = false;

  await ensureVendorDirectory(paths.vendorDir);
  await cleanupInstallTemps(paths);

  try {
    const overridePath = env.SMOKEPOD_BINARY;
    if (overridePath) {
      attemptedSource = overridePath;
      await validateReadableFile(overridePath);
      await copyFile(overridePath, paths.tempPath);
      await finalizeInstall(paths, platform);
      installSucceeded = true;
      return paths.finalPath;
    }

    attemptedSource = getAttemptedReleaseSource(version, platform, arch);

    if (isDevVersion(version)) {
      throw new Error('source-control development version cannot be installed from GitHub releases without SMOKEPOD_BINARY');
    }

    const target = getReleasePlatform(platform, arch);
    const download = await downloadVerifiedBinary(version, target, requestImpl);
    attemptedSource = download.binaryUrl;

    await writeFile(paths.downloadPath, download.binaryBuffer);
    await rename(paths.downloadPath, paths.tempPath);
    await finalizeInstall(paths, platform);
    installSucceeded = true;

    return paths.finalPath;
  } catch (error) {
    throw new Error(
      formatInstallError({
        detectedPlatform: platform,
        detectedArch: arch,
        attemptedSource,
        reason: error.message,
        version
      })
    );
  } finally {
    await cleanupInstallTemps(paths, { removeBackup: installSucceeded });
  }
}

export async function main(options = {}) {
  return installBinary({
    packageRoot: path.resolve(options.packageRoot || PACKAGE_ROOT),
    env: options.env || process.env,
    platform: options.platform || process.platform,
    arch: options.arch || process.arch,
    requestImpl: options.requestImpl || requestBuffer
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
