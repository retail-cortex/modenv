// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { run } from 'node:test';
import { spec } from 'node:test/reporters';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Search for compiled test files
function findTestFile(filename) {
  const candidates = [
    path.join(__dirname, filename),
    path.join(__dirname, '..', 'dist', 'test', filename),
    path.join(__dirname, '..', 'dist', 'clients', 'typescript', 'test', filename),
    path.join(__dirname, 'dist', 'test', filename),
  ];
  for (const c of candidates) {
    if (fs.existsSync(c)) {
      return path.resolve(c);
    }
  }
  // Try recursive search in parent
  function search(dir) {
    if (!fs.existsSync(dir)) return null;
    const entries = fs.readdirSync(dir, { withFileTypes: true });
    for (const e of entries) {
      const full = path.join(dir, e.name);
      if (e.isDirectory() && !e.name.startsWith('.') && e.name !== 'node_modules') {
        const found = search(full);
        if (found) return found;
      } else if (e.isFile() && e.name === filename) {
        return full;
      }
    }
    return null;
  }
  const found = search(path.resolve(__dirname, '..'));
  if (found) return found;
  throw new Error(`Could not find ${filename}. Searched: ${candidates.join(', ')}`);
}

const files = [
  findTestFile('modenv.test.js'),
  findTestFile('integration.test.js'),
];

const stream = run({ files });
stream.compose(new spec()).pipe(process.stdout);
stream.on('test:fail', () => {
  process.exitCode = 1;
});
