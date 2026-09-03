---
title: "Integration Guide"
weight: 1
---

# TypeScript Integration Guide

## Installation

### With Bazel
Add `modenv` as a dependency in your `BUILD.bazel`:
```python
load("@aspect_rules_js//js:defs.bzl", "js_library")

js_library(
    name = "app",
    srcs = ["main.js"],
    deps = ["//clients/typescript:typescript_sources"],
)
```

### With pnpm / npm
```bash
pnpm add @retail-cortex/modenv
```

## Basic Configuration Loading

```typescript
import { load } from "@retail-cortex/modenv";

const config = load();
console.log(`Application: ${config.app_name}`);
console.log(`Database Host: ${config.database.host}`);
```

## Type-Safe Object Binding with Defensive Copying

```typescript
import { load } from "@retail-cortex/modenv";

interface DatabaseConfig {
  host: string;
  user: string;
  password: string; // Values with xor: are decrypted automatically
}

interface AppConfig {
  app_name: string;
  port: number;
  features: string[];
  database: DatabaseConfig;
}

const template: AppConfig = {
  app_name: "",
  port: 8080,
  features: [],
  database: { host: "", user: "", password: "" },
};

// Returns an isolated, defensive clone matching the interface
const config = load<AppConfig>(template);

console.log(`Starting ${config.app_name} on port ${config.port}`);
console.log(`Connected to ${config.database.host}`);
```

## Environment Variable Lifecycle

```typescript
import { EnvManager } from "@retail-cortex/modenv";

const manager = new EnvManager();

// Track and set environment variable
manager.set("MODENV_RUNTIME", "production");
const [val, exists] = manager.lookup("MODENV_RUNTIME");
console.log(`Runtime: ${val} (active: ${exists})`);

// Revert environment to its original state
manager.restore();
```

## Running Tests with Bazel

```bash
bazel test //clients/typescript/...
```
