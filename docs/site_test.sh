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

set -euxo pipefail

# Assert that generated site files exist
test -f docs/site/index.html
test -f docs/site/docs/index.html
test -f docs/site/docs/purpose/index.html
test -f docs/site/docs/architecture/index.html
test -f docs/site/docs/hierarchy/index.html
test -f docs/site/docs/go/index.html
test -f docs/site/docs/python/index.html
test -f docs/site/docs/java/index.html
test -f docs/site/docs/typescript/index.html
test -f docs/site/docs/cli/index.html

# Verify mermaid hooks and initialization are injected
grep -q 'class="mermaid"' docs/site/index.html
grep -q 'mermaid.initialize' docs/site/index.html
grep -q 'class="mermaid"' docs/site/docs/architecture/index.html
grep -q 'class="mermaid"' docs/site/docs/hierarchy/index.html
