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

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import * as path from "node:path";
import * as fs from "node:fs";
import { EnvManager, load } from "../src/index.js";

const getTestConfigsDir = (): string => {
  let curr = process.cwd();
  while (curr) {
    const candidate = path.join(curr, "test", "configs");
    if (fs.existsSync(candidate) && fs.statSync(candidate).isDirectory()) {
      return candidate;
    }
    const parent = path.dirname(curr);
    if (parent === curr) break;
    curr = parent;
  }
  return path.resolve("test/configs");
};

describe("modenv TypeScript Integration", () => {
  it("test integration env manager", () => {
    const manager = new EnvManager();
    delete process.env.INTEGRATION_KEY_1;
    delete process.env.INTEGRATION_KEY_2;

    manager.set("INTEGRATION_KEY_1", "val1");
    manager.set("INTEGRATION_KEY_2", "val2");

    assert.equal(process.env.INTEGRATION_KEY_1, "val1");
    assert.equal(process.env.INTEGRATION_KEY_2, "val2");

    manager.restore();

    assert.equal("INTEGRATION_KEY_1" in process.env, false);
    assert.equal("INTEGRATION_KEY_2" in process.env, false);
  });

  it("test integration load", () => {
    const dir = path.join(getTestConfigsDir(), "integration");
    process.env.MODENV_PREFIX = dir;

    interface IntegrationConfig {
      integration_val: string;
      port: number;
    }

    const cfg: IntegrationConfig = {
      integration_val: "",
      port: 0,
    };

    const clone = load<IntegrationConfig>(cfg);

    assert.equal(cfg.integration_val, "base");
    assert.equal(cfg.port, 2000);

    assert.equal(clone.integration_val, "base");
    assert.equal(clone.port, 2000);
  });
});
