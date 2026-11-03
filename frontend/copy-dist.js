import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const srcDist = path.resolve(__dirname, 'dist');
const targetDist = path.resolve(__dirname, '../cmd/sweep-gui/frontend/dist');

if (fs.existsSync(srcDist)) {
  fs.mkdirSync(targetDist, { recursive: true });
  fs.cpSync(srcDist, targetDist, { recursive: true });
  console.log(`[copy-dist] Synced build assets to ${targetDist}`);
}
