#!/usr/bin/env bash
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

if ! command -v node &> /dev/null; then
  for p in "$HOME/.nvm/versions/node"/*/bin "$HOME/.asdf/shims" "$HOME/.volta/bin" /opt/homebrew/bin /usr/local/bin /usr/bin; do
    if [ -x "$p/node" ]; then
      export PATH="$p:$PATH"
      break
    fi
  done
fi

if [ -d "clients/typescript" ]; then
  cd clients/typescript
elif [ -d "typescript" ]; then
  cd typescript
fi

if [ ! -f "./node_modules/typescript/bin/tsc" ]; then
  echo "Error: typescript dependencies are missing. Run 'cd clients/typescript && pnpm install' before running tests." >&2
  exit 1
fi

node ./node_modules/typescript/bin/tsc
node --test dist/test/modenv.test.js dist/test/integration.test.js
