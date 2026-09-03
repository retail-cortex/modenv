---
title: "TypeScript"
weight: 40
---

# TypeScript Implementation

The TypeScript client library of `modenv` is located in `clients/typescript/`. It provides full TypeScript definitions, NodeNext ESM compatibility, zero-dependency TOML parsing via `smol-toml`, and native Node.js test execution.

## Directory Layout
- `clients/typescript/src/`:
  - `modenv.ts`: Core configuration loader and cryptographic functions.
  - `index.ts`: Module entrypoint.
- `clients/typescript/test/`: Unit and integration test suites using `node:test`.
- `clients/typescript/package.json`: NPM package descriptor.

See [TypeScript Integration Guide](integration/) for usage instructions and code examples.
