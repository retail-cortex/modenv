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

import * as child_process from "node:child_process";
import * as crypto from "node:crypto";
import * as fs from "node:fs";
import * as path from "node:path";
import { parse } from "smol-toml";

export interface SecretsStoreConfig {
  type?: string;
  google_cloud_project?: string;
  google_cloud_region?: string;
  key_location?: string;
}

export type CloudSecretResolver = (uri: string, cfg: SecretsStoreConfig) => string;
let customCloudSecretResolver: CloudSecretResolver | null = null;
export const setCloudSecretResolver = (resolver: CloudSecretResolver | null): void => {
  customCloudSecretResolver = resolver;
};

/**
 * Tracks changes to environment variables and allows reverting them to their original states.
 */
export class EnvManager {
  private original: Map<string, string | undefined> = new Map();

  public set = (key: string, value: string): void => {
    if (!this.original.has(key)) {
      this.original.set(key, process.env[key]);
    }
    process.env[key] = value;
  };

  public get = (key: string): string | undefined => {
    return process.env[key];
  };

  public lookup = (key: string): [string, boolean] => {
    if (key in process.env && process.env[key] !== undefined) {
      return [process.env[key]!, true];
    }
    return ["", false];
  };

  public unset = (key: string): void => {
    if (!this.original.has(key)) {
      this.original.set(key, process.env[key]);
    }
    delete process.env[key];
  };

  public restore = (): void => {
    for (const [key, origVal] of this.original.entries()) {
      if (origVal === undefined) {
        delete process.env[key];
      } else {
        process.env[key] = origVal;
      }
    }
    this.original.clear();
  };
}

/**
 * Encrypts a plain text string using XOR and returns a hex encoded string prefixed with 'simple://'.
 */
export const encryptSecret = (plainText: string): string => {
  const key = getSecretKey();
  const keyBytes = Buffer.from(key, "utf-8");
  const inputBytes = Buffer.from(plainText, "utf-8");
  const output = Buffer.alloc(inputBytes.length);

  for (let i = 0; i < inputBytes.length; i++) {
    output[i] = inputBytes[i] ^ keyBytes[i % keyBytes.length];
  }

  return "simple://" + output.toString("hex");
};

/**
 * Encrypts a plain text string using XOR and returns a hex encoded string prefixed with legacy 'xor:'.
 */
export const encryptLegacySecret = (plainText: string): string => {
  const key = getSecretKey();
  const keyBytes = Buffer.from(key, "utf-8");
  const inputBytes = Buffer.from(plainText, "utf-8");
  const output = Buffer.alloc(inputBytes.length);

  for (let i = 0; i < inputBytes.length; i++) {
    output[i] = inputBytes[i] ^ keyBytes[i % keyBytes.length];
  }

  return "xor:" + output.toString("hex");
};

/**
 * Decrypts an encoded string starting with 'simple://' or 'xor:' and returns the plain text.
 */
export const decryptSecret = (encodedText: string): string => {
  if (!encodedText || typeof encodedText !== "string") {
    throw new Error("secret cannot be null");
  }

  let hexStr: string;
  if (encodedText.startsWith("simple://")) {
    hexStr = encodedText.slice(9);
  } else if (encodedText.startsWith("xor:")) {
    hexStr = encodedText.slice(4);
  } else {
    throw new Error("invalid secret format: missing 'simple://' or 'xor:' prefix");
  }

  if (hexStr.length % 2 !== 0 || !/^[0-9a-fA-F]*$/.test(hexStr)) {
    throw new Error("failed to hex decode secret: invalid hex string");
  }
  const data = Buffer.from(hexStr, "hex");
  const key = getSecretKey();
  const keyBytes = Buffer.from(key, "utf-8");
  const output = Buffer.alloc(data.length);

  for (let i = 0; i < data.length; i++) {
    output[i] = data[i] ^ keyBytes[i % keyBytes.length];
  }

  return output.toString("utf-8");
};

/**
 * Encrypts a plain text string using an RSA public key PEM and PKCS#1 v1.5 padding.
 */
export const encryptPKSSecret = (plainText: string, pubKeyPEM: string): string => {
  const buffer = Buffer.from(plainText, "utf-8");
  const encrypted = crypto.publicEncrypt(
    {
      key: pubKeyPEM,
      padding: crypto.constants.RSA_PKCS1_PADDING,
    },
    buffer
  );
  return "pks://" + encrypted.toString("base64");
};

/**
 * Decrypts a 'pks://' prefixed base64 RSA encrypted secret.
 */
export const decryptPKSSecret = (encodedText: string, cfg?: SecretsStoreConfig): string => {
  if (!encodedText || !encodedText.startsWith("pks://")) {
    throw new Error("invalid secret format: missing 'pks://' prefix");
  }
  const rawB64 = encodedText.slice(6);

  let pemData = process.env.MODENV_PRIVATE_KEY;
  if (!pemData) {
    let keyPath = process.env.MODENV_KEY_LOCATION || cfg?.key_location;
    if (!keyPath) {
      throw new Error(
        "cannot decrypt PKS secret: no private key found in key_location, MODENV_KEY_LOCATION, or MODENV_PRIVATE_KEY"
      );
    }
    let targetPath = keyPath;
    if (!path.isAbsolute(targetPath) && !fs.existsSync(targetPath)) {
      const prefix = process.env.MODENV_PREFIX;
      if (prefix) {
        targetPath = path.join(prefix, targetPath);
      }
    }
    if (!fs.existsSync(targetPath) || fs.statSync(targetPath).isDirectory()) {
      throw new Error(`could not read private key file at ${targetPath}`);
    }
    pemData = fs.readFileSync(targetPath, "utf-8");
  }

  const buffer = Buffer.from(rawB64, "base64");
  const decrypted = crypto.privateDecrypt(
    {
      key: pemData,
      padding: crypto.constants.RSA_PKCS1_PADDING,
    },
    buffer
  );
  return decrypted.toString("utf-8");
};

/**
 * Resolves a secret from Google Cloud Secret Manager URI.
 */
export const resolveCloudSecret = (uri: string, cfg?: SecretsStoreConfig): string => {
  if (!uri || !uri.startsWith("cloud://")) {
    throw new Error(`invalid cloud secret URI: ${uri}`);
  }

  const trimmed = uri.slice(8);
  let project = "";
  let secret = "";
  let version = "latest";

  const match = trimmed.match(/^projects\/([^/]+)\/secrets\/([^/]+)(?:\/versions\/([^/]+))?$/);
  if (match) {
    project = match[1];
    secret = match[2];
    if (match[3]) {
      version = match[3];
    }
  } else {
    if (trimmed.includes(":")) {
      const parts = trimmed.split(":");
      secret = parts[0];
      version = parts[1];
    } else {
      secret = trimmed;
    }
    if (cfg?.google_cloud_project) {
      project = cfg.google_cloud_project;
    }
    if (!project) {
      project = process.env.GOOGLE_CLOUD_PROJECT || process.env.GCP_PROJECT || process.env.MODENV_GCP_PROJECT || "";
    }
  }

  if (!project) {
    throw new Error(`cannot resolve cloud secret ${uri}: Google Cloud Project ID is not configured`);
  }

  if (!secret) {
    throw new Error(`cloud secret URI missing secret name: ${uri}`);
  }
  if (/[/ \r\n]/.test(secret) || /[/ \r\n]/.test(project) || /[/ \r\n]/.test(version)) {
    throw new Error(`invalid secret, project, or version identifier in cloud secret URI: ${uri}`);
  }

  if (customCloudSecretResolver) {
    return customCloudSecretResolver(uri, cfg || {});
  }

  const overrideKey = `MODENV_CLOUD_SECRET_${secret.toUpperCase().replace(/-/g, "_")}`;
  if (process.env[overrideKey]) {
    return process.env[overrideKey]!;
  }

  const token = process.env.GOOGLE_OAUTH_ACCESS_TOKEN || process.env.GCP_ACCESS_TOKEN || "";
  if (!token) {
    const cleanSecret = secret.toUpperCase().replace(/-/g, "_");
    throw new Error(
      `failed to resolve cloud secret '${secret}': no Google Cloud credentials or MODENV_CLOUD_SECRET_${cleanSecret} found`
    );
  }
  if (/[\r\n]/.test(token)) {
    throw new Error("invalid OAuth token: contains newline characters");
  }

  const baseURL = process.env.MODENV_GCP_ENDPOINT || "https://secretmanager.googleapis.com";
  const apiURL = `${baseURL}/v1/projects/${project}/secrets/${secret}/versions/${version}:access`;

  const curlRes = child_process.spawnSync("curl", ["-s", "-f", "-H", `Authorization: Bearer ${token}`, apiURL], {
    encoding: "utf-8",
  });
  if (curlRes.status !== 0 || !curlRes.stdout) {
    throw new Error(`failed to fetch cloud secret '${secret}': ${curlRes.stderr || "HTTP request failed"}`);
  }
  let payload: any;
  try {
    payload = JSON.parse(curlRes.stdout);
  } catch (e: any) {
    throw new Error(`failed to parse Secret Manager response: ${e.message}`);
  }
  const dataB64 = payload?.payload?.data;
  if (!dataB64) {
    throw new Error("missing payload.data in Secret Manager response");
  }
  return Buffer.from(dataB64, "base64").toString("utf-8");
};

const getSecretKey = (): string => {
  return process.env.MODENV_KEY || "modenv-default-key";
};

export const resolvePath = (filename: string): string => {
  const prefix = process.env.MODENV_PREFIX;
  if (!prefix) {
    return filename;
  }
  return path.join(prefix, filename);
};

const deepMerge = (dst: Record<string, any>, src: Record<string, any>): Record<string, any> => {
  for (const key of Object.keys(src)) {
    const srcVal = src[key];
    const dstVal = dst[key];
    if (
      dstVal &&
      typeof dstVal === "object" &&
      !Array.isArray(dstVal) &&
      srcVal &&
      typeof srcVal === "object" &&
      !Array.isArray(srcVal)
    ) {
      deepMerge(dstVal, srcVal);
    } else {
      dst[key] = srcVal;
    }
  }
  return dst;
};

const decryptConfigInPlace = (obj: any, cfg?: SecretsStoreConfig): void => {
  if (!obj || typeof obj !== "object") return;

  if (Array.isArray(obj)) {
    for (let i = 0; i < obj.length; i++) {
      const val = obj[i];
      if (typeof val === "string") {
        if (val.startsWith("simple://") || val.startsWith("xor:")) {
          obj[i] = decryptSecret(val);
        } else if (val.startsWith("pks://")) {
          obj[i] = decryptPKSSecret(val, cfg);
        } else if (val.startsWith("cloud://")) {
          obj[i] = resolveCloudSecret(val, cfg);
        }
      } else if (typeof val === "object") {
        decryptConfigInPlace(val, cfg);
      }
    }
  } else {
    for (const key of Object.keys(obj)) {
      const val = obj[key];
      if (typeof val === "string") {
        if (val.startsWith("simple://") || val.startsWith("xor:")) {
          obj[key] = decryptSecret(val);
        } else if (val.startsWith("pks://")) {
          obj[key] = decryptPKSSecret(val, cfg);
        } else if (val.startsWith("cloud://")) {
          obj[key] = resolveCloudSecret(val, cfg);
        }
      } else if (typeof val === "object") {
        decryptConfigInPlace(val, cfg);
      }
    }
  }
};

/**
 * Loads and parses hierarchical TOML environment configurations:
 * 1. .env.toml (required)
 * 2. .env.${MODENV_RUNTIME}.toml (optional runtime override)
 * 3. .env.local.toml (optional local override, loaded last)
 *
 * Paths are resolved relative to MODENV_PREFIX if configured.
 * Returns a defensive copy of the merged configuration.
 */
export const load = <T = Record<string, any>>(target?: T): T => {
  const baseFile = resolvePath(".env.toml");
  if (!fs.existsSync(baseFile) || fs.statSync(baseFile).isDirectory()) {
    throw new Error(`failed to load base config ${baseFile}: file not found`);
  }

  const baseContent = fs.readFileSync(baseFile, "utf-8");
  let merged: Record<string, any> = parse(baseContent) as Record<string, any>;

  const runtime = process.env.MODENV_RUNTIME;
  if (runtime) {
    const runtimeFile = resolvePath(`.env.${runtime}.toml`);
    if (fs.existsSync(runtimeFile) && !fs.statSync(runtimeFile).isDirectory()) {
      const runtimeContent = fs.readFileSync(runtimeFile, "utf-8");
      const runtimeMap = parse(runtimeContent) as Record<string, any>;
      merged = deepMerge(merged, runtimeMap);
    }
  }

  const localFile = resolvePath(".env.local.toml");
  if (fs.existsSync(localFile) && !fs.statSync(localFile).isDirectory()) {
    const localContent = fs.readFileSync(localFile, "utf-8");
    const localMap = parse(localContent) as Record<string, any>;
    merged = deepMerge(merged, localMap);
  }

  let cfg: SecretsStoreConfig = {};
  if (merged.secrets_store && typeof merged.secrets_store === "object") {
    cfg = {
      type: String(merged.secrets_store.type || "simple"),
      google_cloud_project: String(merged.secrets_store.google_cloud_project || ""),
      google_cloud_region: String(merged.secrets_store.google_cloud_region || ""),
      key_location: String(merged.secrets_store.key_location || ""),
    };
  }

  decryptConfigInPlace(merged, cfg);

  if (target === undefined || target === null) {
    return structuredClone(merged) as T;
  }

  if (typeof target === "object") {
    bindToObject(target, merged);
    return structuredClone(target);
  }

  return structuredClone(merged) as T;
};

const bindToObject = (target: any, data: any): any => {
  if (!target || typeof target !== "object" || !data || typeof data !== "object") {
    return target;
  }
  for (const [key, value] of Object.entries(data)) {
    if (
      value !== null &&
      typeof value === "object" &&
      !Array.isArray(value) &&
      key in target &&
      target[key] !== null &&
      typeof target[key] === "object" &&
      !Array.isArray(target[key])
    ) {
      bindToObject(target[key], value);
    } else {
      target[key] = structuredClone(value);
    }
  }
  return target;
};
