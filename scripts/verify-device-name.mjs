#!/usr/bin/env node
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(fileURLToPath(new URL('..', import.meta.url)));
const read = (file) => readFileSync(resolve(root, file), 'utf8');

const settings = read('src/config/settings.go');
const rootCmd = read('src/cmd/root.go');
const app = read('src/usecase/app.go');

assert.match(settings, /AppOs\s*=\s*"Retena"/, 'default visible app OS/device name must be Retena');
assert.doesNotMatch(settings, /AppOs\s*=\s*"(?:Chrome|GOWA)"/, 'default visible app OS/device name must not be Chrome or GOWA');
assert.match(rootCmd, /config\.AppOs[\s\S]*`os name --os <string> \| example: --os="Retena"`/, '--os help must show Retena while preserving override behavior');
assert.match(app, /PairPhone\(ctx, phoneNumber, true, whatsmeow\.PairClientChrome, "Retena"\)/, 'phone-code pairing must keep protocol compatibility but show Retena as the linked-device label');
assert.doesNotMatch(app, /Chrome \(Linux\)/, 'phone-code pairing must not show Chrome (Linux)');

console.log('verify-device-name OK');
