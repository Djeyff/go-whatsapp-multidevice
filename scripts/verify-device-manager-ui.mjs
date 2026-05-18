#!/usr/bin/env node
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const componentPath = join(here, '..', 'src', 'views', 'components', 'DeviceManager.js');
const source = readFileSync(componentPath, 'utf8');

const checks = [
  ['defaults to hiding disconnected devices', /showDisconnectedDevices:\s*false/.test(source)],
  ['computes visibleDevices from filtered device list', /visibleDevices\(\)\s*\{[\s\S]*showDisconnectedDevices[\s\S]*deviceList\.filter/.test(source)],
  ['computes hidden disconnected count', /hiddenDisconnectedCount\(\)\s*\{[\s\S]*hiddenDisconnectedDevices/.test(source)],
  ['renders filtered device list', /v-for="dev in visibleDevices"/.test(source)],
  ['offers a disconnected toggle', /showDisconnectedDevices\s*=\s*!showDisconnectedDevices/.test(source)],
  ['preserves selected disconnected device visibility', /selectedDeviceId\s*===\s*deviceId/.test(source)],
];

const failures = checks.filter(([, ok]) => !ok);
if (failures.length) {
  console.error('device_manager_ui=failed');
  for (const [name] of failures) console.error(`missing: ${name}`);
  process.exit(1);
}

console.log('device_manager_ui=ok');
