# @retail-cortex/modenv

Hierarchical TOML configuration, secret decryption, and environment loader for TypeScript.

Part of the [modenv](https://github.com/retail-cortex/modenv) multi-language ecosystem.

## Installation

```bash
pnpm add @retail-cortex/modenv
```

## Quickstart

```typescript
import { load } from "@retail-cortex/modenv";

interface AppConfig {
  app_name: string;
  port: number;
  database: {
    host: string;
    password: string; // "xor:..." encrypted secrets are decrypted automatically
  };
}

const config = load<AppConfig>();
console.log(`Server started on port ${config.port}`);
```

## License

Apache-2.0
