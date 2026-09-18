#!/usr/bin/env node
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(fileURLToPath(new URL('..', import.meta.url)));
const part = String(process.argv[2] || '').toLowerCase();
assert.match(part, /^(major|minor|patch)$/, 'usage: bump-retena-product-version.mjs <major|minor|patch>');

const versionPath = resolve(root, 'PRODUCT_VERSION');
const current = readFileSync(versionPath, 'utf8').trim();
const match = current.match(/^(\d+)\.(\d+)(?:\.(\d+))?$/);
assert.ok(match, `PRODUCT_VERSION must be X.Y or X.Y.Z, got ${current}`);
let [major, minor, patch] = [Number(match[1]), Number(match[2]), Number(match[3] || 0)];
if (part === 'major') {
  major += 1;
  minor = 0;
  patch = 0;
} else if (part === 'minor') {
  minor += 1;
  patch = 0;
} else {
  patch += 1;
}
const next = `${major}.${minor}.${patch}`;

const rewriteLiteral = (rel, from, to) => {
  const path = resolve(root, rel);
  const before = readFileSync(path, 'utf8');
  assert.ok(before.includes(from), `${rel} must contain ${from}`);
  writeFileSync(path, before.split(from).join(to));
};

writeFileSync(versionPath, `${next}\n`);
rewriteLiteral(
  'src/config/settings.go',
  `RetenaProductVersion                    = "${current}"`,
  `RetenaProductVersion                    = "${next}"`
);
rewriteLiteral(
  'src/config/settings_test.go',
  `RetenaProductVersion != "${current}"`,
  `RetenaProductVersion != "${next}"`
);
rewriteLiteral(
  'src/config/settings_test.go',
  `want ${current}`,
  `want ${next}`
);

console.log(`retena_product_version ${current} -> ${next}`);
