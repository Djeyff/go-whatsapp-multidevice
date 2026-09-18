#!/usr/bin/env node
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(fileURLToPath(new URL('..', import.meta.url)));
const read = (file) => readFileSync(resolve(root, file), 'utf8');

const settings = read('src/config/settings.go');
const linked = read('src/config/linked_device.go');
const rootCmd = read('src/cmd/root.go');
const app = read('src/usecase/app.go');
const initWa = read('src/infrastructure/whatsapp/init.go');
const deviceManager = read('src/infrastructure/whatsapp/device_manager.go');
const productVersion = read('PRODUCT_VERSION').trim();

assert.match(productVersion, /^\d+\.\d+(?:\.\d+)?$/, 'PRODUCT_VERSION must be X.Y or X.Y.Z');
assert.match(settings, new RegExp(`RetenaProductVersion\\s*=\\s*"${productVersion}"`), 'settings.go must match PRODUCT_VERSION');
assert.match(settings, /AppOs\s*=\s*"Retena"/, 'default visible app OS/device name must be Retena');
assert.doesNotMatch(settings, /AppOs\s*=\s*"(?:Chrome|GOWA)"/, 'default visible app OS/device name must not be Chrome or GOWA');
assert.match(linked, /return "Retena Blue " \+ version/, 'blue lane must advertise Retena Blue vXX.XX');
assert.match(linked, /return "Retena " \+ version/, 'main lane must advertise Retena vXX.XX');
assert.match(linked, /genericLinkedDeviceOsChrome/, 'Chrome APP_OS must be ignored for the linked-device label');
assert.match(rootCmd, /config\.ApplyLinkedDeviceOsOverride\(envOs\)/, 'APP_OS must go through the Chrome/GOWA ignore helper');
assert.match(initWa, /config\.LinkedDeviceDisplayName\(\)/, 'QR login must publish the Retena product linked-device name');
assert.match(deviceManager, /config\.LinkedDeviceDisplayName\(\)/, 'device manager must publish the Retena product linked-device name');
assert.match(app, /config\.LinkedDeviceDisplayName\(\)/, 'phone-code pairing must show the Retena product linked-device name');
assert.doesNotMatch(app, /Chrome \(Linux\)/, 'phone-code pairing must not show Chrome (Linux)');
assert.doesNotMatch(initWa, /fmt\.Sprintf\("%s %s", config\.AppOs, config\.AppVersion\)/, 'QR login must not concatenate APP_OS + GOWA AppVersion');
assert.doesNotMatch(deviceManager, /fmt\.Sprintf\("%s %s", config\.AppOs, config\.AppVersion\)/, 'device manager must not concatenate APP_OS + GOWA AppVersion');

console.log('verify-device-name OK');
