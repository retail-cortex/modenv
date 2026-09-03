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

import { describe, it, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import * as path from "node:path";
import * as fs from "node:fs";
import * as os from "node:os";
import * as http from "node:http";
import * as child_process from "node:child_process";
import {
  EnvManager,
  SecretsStoreConfig,
  decryptPKSSecret,
  decryptSecret,
  encryptLegacySecret,
  encryptPKSSecret,
  encryptSecret,
  load,
  resolveCloudSecret,
  resolvePath,
  setCloudSecretResolver,
} from "../src/index.js";

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

describe("modenv TypeScript", () => {
  let originalEnv: NodeJS.ProcessEnv;

  beforeEach(() => {
    originalEnv = { ...process.env };
  });

  afterEach(() => {
    process.env = { ...originalEnv };
    setCloudSecretResolver(null);
  });

  it("test secret encryption decryption roundtrip", () => {
    const plain = "my-super-secret-password-123!";

    // Simple prefix
    const encodedSimple = encryptSecret(plain);
    assert.ok(encodedSimple.startsWith("simple://"));
    const decodedSimple = decryptSecret(encodedSimple);
    assert.equal(decodedSimple, plain);

    // Legacy xor prefix
    const encodedLegacy = encryptLegacySecret(plain);
    assert.ok(encodedLegacy.startsWith("xor:"));
    const decodedLegacy = decryptSecret(encodedLegacy);
    assert.equal(decodedLegacy, plain);

    // Custom key
    process.env.MODENV_KEY = "custom-super-long-key-spec";
    const encodedCustom = encryptSecret(plain);
    assert.notEqual(encodedSimple, encodedCustom);
    const decodedCustom = decryptSecret(encodedCustom);
    assert.equal(decodedCustom, plain);

    const encodedLegacyCustom = encryptLegacySecret(plain);
    assert.notEqual(encodedLegacy, encodedLegacyCustom);
    const decodedLegacyCustom = decryptSecret(encodedLegacyCustom);
    assert.equal(decodedLegacyCustom, plain);
  });

  it("test pks encryption decryption roundtrip", () => {
    const configsDir = getTestConfigsDir();
    const pubPath = path.join(configsDir, "test_rsa_public.pem");
    const privPath = path.join(configsDir, "test_rsa_private.pem");

    const pubPEM = fs.readFileSync(pubPath, "utf-8");
    const privPEM = fs.readFileSync(privPath, "utf-8");

    const secret = "super-secret-rsa-payload";
    const encrypted = encryptPKSSecret(secret, pubPEM);
    assert.ok(encrypted.startsWith("pks://"));

    // Decrypt using MODENV_PRIVATE_KEY
    process.env.MODENV_PRIVATE_KEY = privPEM;
    assert.equal(decryptPKSSecret(encrypted), secret);
    delete process.env.MODENV_PRIVATE_KEY;

    // Decrypt using MODENV_KEY_LOCATION
    process.env.MODENV_KEY_LOCATION = privPath;
    assert.equal(decryptPKSSecret(encrypted), secret);
    delete process.env.MODENV_KEY_LOCATION;

    // Decrypt using SecretsStoreConfig
    const cfg: SecretsStoreConfig = { type: "pks", key_location: privPath };
    assert.equal(decryptPKSSecret(encrypted, cfg), secret);

    // Decrypt using relative key_location with MODENV_PREFIX
    const relCfg: SecretsStoreConfig = { type: "pks", key_location: "test_rsa_private.pem" };
    process.env.MODENV_PREFIX = configsDir;
    assert.equal(decryptPKSSecret(encrypted, relCfg), secret);
  });

  it("test pks errors", () => {
    const configsDir = getTestConfigsDir();
    const pubPath = path.join(configsDir, "test_rsa_public.pem");
    const pubPEM = fs.readFileSync(pubPath, "utf-8");

    // Invalid prefix
    assert.throws(() => {
      decryptPKSSecret("xor:1234");
    });

    // Missing key
    assert.throws(() => {
      decryptPKSSecret("pks://aaaa", { type: "pks" });
    });

    // Nonexistent key file
    assert.throws(() => {
      decryptPKSSecret("pks://aaaa", { type: "pks", key_location: "/no/such/file.pem" });
    });

    // Directory path as key
    assert.throws(() => {
      decryptPKSSecret("pks://aaaa", { type: "pks", key_location: configsDir });
    });

    // Bad PEM
    process.env.MODENV_PRIVATE_KEY = "not a valid pem";
    assert.throws(() => {
      decryptPKSSecret("pks://aaaa");
    });
    delete process.env.MODENV_PRIVATE_KEY;

    // Bad public key during encryption
    assert.throws(() => {
      encryptPKSSecret("val", "invalid pem block");
    });
  });

  it("test cloud secret with env override", () => {
    process.env.MODENV_CLOUD_SECRET_DB_PASS = "env-override-password";
    const cfg: SecretsStoreConfig = { type: "cloud", google_cloud_project: "my-proj" };

    const val = resolveCloudSecret("cloud://db-pass", cfg);
    assert.equal(val, "env-override-password");

    // Test full URI
    process.env.MODENV_CLOUD_SECRET_FULL_SECRET = "full-secret-val";
    const val2 = resolveCloudSecret("cloud://projects/my-proj/secrets/full-secret/versions/1", cfg);
    assert.equal(val2, "full-secret-val");
  });

  it("test cloud secret with custom resolver", () => {
    setCloudSecretResolver((uri, cfg) => {
      return `custom-${uri}-${cfg.google_cloud_project}`;
    });

    const val = resolveCloudSecret("cloud://some-secret", {
      type: "cloud",
      google_cloud_project: "my-proj",
    });
    assert.equal(val, "custom-cloud://some-secret-my-proj");

    setCloudSecretResolver(null);
  });

  it("test cloud secret with mock endpoint", async () => {
    const serverScript = `
const http = require("http");
const server = http.createServer((req, res) => {
  if (req.headers.authorization !== "Bearer mock-token-123") {
    res.writeHead(401);
    res.end("unauthorized");
    return;
  }
  if (req.url === "/v1/projects/my-proj/secrets/db-pass/versions/latest:access") {
    res.writeHead(200, { "Content-Type": "application/json" });
    const b64Data = Buffer.from("server-secret-42").toString("base64");
    res.end(JSON.stringify({ payload: { data: b64Data } }));
  } else if (req.url === "/v1/projects/my-proj/secrets/bad-json/versions/latest:access") {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end("{ invalid json");
  } else if (req.url === "/v1/projects/my-proj/secrets/missing-data/versions/latest:access") {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ payload: {} }));
  } else {
    res.writeHead(404);
    res.end("not found");
  }
});
server.listen(0, "127.0.0.1", () => {
  process.stdout.write(server.address().port + "\\n");
});
`;
    const child = child_process.spawn(process.execPath, ["-e", serverScript]);
    const port = await new Promise<number>((resolve, reject) => {
      let buf = "";
      child.stdout.on("data", (d) => {
        buf += d.toString();
        if (buf.includes("\n")) {
          resolve(parseInt(buf.trim(), 10));
        }
      });
      child.on("error", reject);
    });

    try {
      process.env.MODENV_GCP_ENDPOINT = `http://127.0.0.1:${port}`;
      process.env.GOOGLE_OAUTH_ACCESS_TOKEN = "mock-token-123";
      const cfg: SecretsStoreConfig = { type: "cloud", google_cloud_project: "my-proj" };

      const val = resolveCloudSecret("cloud://db-pass", cfg);
      assert.equal(val, "server-secret-42");

      // 404 error
      assert.throws(() => {
        resolveCloudSecret("cloud://not-found", cfg);
      });

      // Bad JSON response
      assert.throws(() => {
        resolveCloudSecret("cloud://bad-json", cfg);
      });

      // Missing payload.data
      assert.throws(() => {
        resolveCloudSecret("cloud://missing-data", cfg);
      });
    } finally {
      child.kill();
    }
  });

  it("test cloud secret errors", () => {
    // Invalid URI
    assert.throws(() => {
      resolveCloudSecret("invalid://uri");
    });

    // Missing project
    delete process.env.GOOGLE_CLOUD_PROJECT;
    delete process.env.GCP_PROJECT;
    assert.throws(() => {
      resolveCloudSecret("cloud://secret", {});
    });

    // No credentials or override
    const cfg: SecretsStoreConfig = { type: "cloud", google_cloud_project: "proj" };
    delete process.env.GOOGLE_OAUTH_ACCESS_TOKEN;
    delete process.env.GCP_ACCESS_TOKEN;
    delete process.env.MODENV_CLOUD_SECRET_UNCONFIGURED_SECRET;
    assert.throws(() => {
      resolveCloudSecret("cloud://unconfigured-secret", cfg);
    });
  });

  it("test load smart secrets directory", () => {
    const configsDir = getTestConfigsDir();
    const smartDir = path.join(configsDir, "smart-secrets");
    const privPath = path.join(configsDir, "test_rsa_private.pem");

    process.env.MODENV_PREFIX = smartDir;
    process.env.MODENV_KEY_LOCATION = privPath;
    process.env.MODENV_CLOUD_SECRET_MY_DB_SECRET = "cloud-resolved-pass";
    process.env.MODENV_CLOUD_SECRET_MY_FULL_SECRET = "full-cloud-pass";

    const cfg = load() as any;
    assert.equal(cfg.app_name, "smart-secrets-app");

    assert.equal(cfg.database.simple_password, "local_db_password");
    assert.equal(cfg.database.legacy_password, "local_db_password");
    assert.equal(cfg.database.pks_password, "pks-decrypted-secret-42");
    assert.equal(cfg.database.cloud_password, "cloud-resolved-pass");
    assert.equal(cfg.database.cloud_full_uri, "full-cloud-pass");
  });

  it("test load with encrypted secrets", () => {
    const dir = path.join(getTestConfigsDir(), "secrets");
    process.env.MODENV_PREFIX = dir;

    const cfg = load();
    assert.equal(cfg.app_name, "secret-app");
    assert.equal(cfg.database.password, "local_db_password");
  });

  it("test load with prefix", () => {
    const dir = path.join(getTestConfigsDir(), "prefix-test");
    process.env.MODENV_PREFIX = dir;

    const cfg = load();
    assert.equal(cfg.app_name, "prefixed-app");
    assert.equal(cfg.port, 2000);
  });

  it("test load map default", () => {
    const dir = path.join(getTestConfigsDir(), "default");
    process.env.MODENV_PREFIX = dir;

    const cfg = load();
    assert.equal(cfg.app_name, "modenv-default");
    assert.equal(cfg.port, 8080);
    assert.equal(cfg.database.host, "localhost");
  });

  it("test load struct", () => {
    const dir = path.join(getTestConfigsDir(), "struct");
    process.env.MODENV_PREFIX = dir;

    interface Config {
      app_name: string;
      port: number;
      features: string[];
      database: { host: string; user: string };
    }

    const cfg: Config = {
      app_name: "",
      port: 0,
      features: [],
      database: { host: "", user: "" },
    };

    const clone = load<Config>(cfg);

    assert.equal(cfg.app_name, "modenv-struct");
    assert.equal(cfg.port, 9000);
    assert.deepEqual(cfg.features, ["web"]);
    assert.equal(cfg.database.host, "db");

    assert.equal(clone.app_name, "modenv-struct");
    assert.equal(clone.port, 9000);
  });

  it("test load runtime override", () => {
    const dir = path.join(getTestConfigsDir(), "runtime");
    process.env.MODENV_PREFIX = dir;
    process.env.MODENV_RUNTIME = "production";

    const cfg = load();
    assert.equal(cfg.app_name, "base");
    assert.equal(cfg.port, 9999);
    assert.equal(cfg.database.host, "prod-db");
  });

  it("test load local override last", () => {
    const dir = path.join(getTestConfigsDir(), "local-override");
    process.env.MODENV_PREFIX = dir;
    process.env.MODENV_RUNTIME = "production";

    const cfg = load();
    assert.equal(cfg.app_name, "prod");
    assert.equal(cfg.port, 7777);
    assert.equal(cfg.database.host, "local-db");
  });

  it("test load defensive copy", () => {
    const dir = path.join(getTestConfigsDir(), "defensive");
    process.env.MODENV_PREFIX = dir;

    const target = {
      app_name: "",
      features: [] as string[],
      database: { host: "" },
    };

    const clone = load(target);

    assert.equal(target.app_name, "defensive");
    assert.equal(clone.app_name, "defensive");

    clone.app_name = "mutated";
    clone.features[0] = "mutated-feature";
    clone.database.host = "mutated-host";

    assert.equal(target.app_name, "defensive");
    assert.equal(target.features[0], "original");
    assert.equal(target.database.host, "original-host");
  });

  it("test load missing base file throws", () => {
    const dir = path.join(getTestConfigsDir(), "missing");
    process.env.MODENV_PREFIX = dir;

    assert.throws(() => {
      load();
    });
  });

  it("test env manager", () => {
    const m = new EnvManager();
    m.set("TEST_NEW_KEY", "new_val");

    const [val, exists] = m.lookup("TEST_NEW_KEY");
    assert.equal(exists, true);
    assert.equal(val, "new_val");

    m.set("TEST_NEW_KEY", "modified_val");
    assert.equal(m.get("TEST_NEW_KEY"), "modified_val");

    m.unset("TEST_NEW_KEY");
    const [, existsAfterUnset] = m.lookup("TEST_NEW_KEY");
    assert.equal(existsAfterUnset, false);

    m.restore();
    assert.equal(m.lookup("TEST_NEW_KEY")[1], false);

    assert.equal(m.get("UNTRACKED_KEY"), undefined);
    m.unset("UNTRACKED_KEY");
    m.restore();
  });

  it("test resolvePath", () => {
    delete process.env.MODENV_PREFIX;
    assert.equal(resolvePath(".env.toml"), ".env.toml");
    process.env.MODENV_PREFIX = "/tmp/modenv-test";
    assert.equal(resolvePath(".env.toml"), path.join("/tmp/modenv-test", ".env.toml"));
    delete process.env.MODENV_PREFIX;
  });

  it("test invalid secret decryption", () => {
    assert.throws(() => {
      decryptSecret(null as any);
    });
    assert.throws(() => {
      decryptSecret("not-prefixed");
    });
    assert.throws(() => {
      decryptSecret("xor:zz-invalid");
    });
    assert.throws(() => {
      decryptSecret("xor:123");
    });
    assert.throws(() => {
      decryptSecret("simple://zz-invalid");
    });
  });

  it("test array secrets and nested structures", () => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "modenv-ts-arr-"));
    process.env.MODENV_PREFIX = tmpDir;

    try {
      const sec1 = encryptSecret("arr-secret-1");
      const sec2 = encryptLegacySecret("arr-secret-2");
      process.env.MODENV_CLOUD_SECRET_ARR_CLOUD = "arr-cloud-val";
      const toml = `
app_name = "arr-app"
secret_list = ["${sec1}", "plain", "cloud://arr-cloud"]
nested_list = [["${sec2}"]]
nested_objs = [{ key = "${sec1}" }]

[secrets_store]
type = "simple"
google_cloud_project = "test-proj"
`;
      fs.writeFileSync(path.join(tmpDir, ".env.toml"), toml);

      const res = load() as any;
      assert.equal(res.secret_list[0], "arr-secret-1");
      assert.equal(res.secret_list[1], "plain");
      assert.equal(res.secret_list[2], "arr-cloud-val");
      assert.equal(res.nested_list[0][0], "arr-secret-2");
      assert.equal(res.nested_objs[0].key, "arr-secret-1");
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  it("test load without prefix", () => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "modenv-ts-noprefix-"));
    const origCwd = process.cwd();
    delete process.env.MODENV_PREFIX;

    try {
      fs.writeFileSync(path.join(tmpDir, ".env.toml"), 'app_name = "no-prefix-ts"\n');
      process.chdir(tmpDir);
      const res = load() as any;
      assert.equal(res.app_name, "no-prefix-ts");
    } finally {
      process.chdir(origCwd);
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  it("test load primitive target", () => {
    const dir = path.join(getTestConfigsDir(), "default");
    process.env.MODENV_PREFIX = dir;

    const res = load(123 as any) as any;
    assert.equal(res.app_name, "modenv-default");
  });
});
