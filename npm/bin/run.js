#!/usr/bin/env node

import { fileURLToPath } from 'node:url';
import path from 'node:path';
import process from 'node:process';
import { getVendorBinaryName } from '../install.mjs';

const BIN_DIR = path.dirname(fileURLToPath(import.meta.url));

function resolveBinaryPath(platform = process.platform) {
  return path.resolve(BIN_DIR, '..', 'vendor', getVendorBinaryName(platform));
}

export async function main() {
  const binaryPath = resolveBinaryPath();
  throw new Error(`smokepod launcher is not implemented yet: expected binary at ${binaryPath}`);
}

if (process.argv[1] && import.meta.url === new URL(process.argv[1], 'file:').href) {
  await main();
}
