---
title: "Upgrading & Migration"
weight: 7
---

# Upgrading from Previous Versions

This guide outlines the architectural enhancements, workspace refactoring, and new secret management capabilities introduced in the latest release of `modenv`.

---

## 1. Smart Secrets & Secret Stores

### Backward Compatibility
Previous versions of `modenv` used a single XOR cipher indicated by the `xor:` prefix (e.g., `xor:010007...`). 

**All legacy `xor:` secrets remain 100% backward compatible.** No changes are required for existing configuration files to continue functioning identically.

### Modernizing to `simple://`
The symmetric XOR cipher has been standardized under the `simple://` URI scheme. To update existing secrets:
```bash
# Encrypt using the modern simple:// scheme
modenv encode --type=simple "my-plaintext-password"
# Output: simple://01000704...

# Legacy xor: format can still be emitted when needed
modenv encode --legacy "my-plaintext-password"
# Output: xor:01000704...
```

### New Secret Schemes
- **Asymmetric RSA (`pks://`)**: Encrypt with an RSA public key in CI/CD or operations, decrypting securely at runtime using an RSA private key specified via `MODENV_PRIVATE_KEY`, `MODENV_KEY_LOCATION`, or `[secrets_store] key_location`.
- **Google Cloud Secret Manager (`cloud://`)**: Directly resolve secrets from GCP Secret Manager using `cloud://<secret-name>` or fully qualified resource paths `cloud://projects/<p>/secrets/<s>/versions/<v>`.

### Optional `[secrets_store]` Configuration
You can now define an optional `[secrets_store]` table at the root level of your `.env.toml`:
```toml
[secrets_store]
type = "simple"                  # Default store: "simple", "pks", or "cloud"
google_cloud_project = "my-gcp-project"
google_cloud_region = "us-central1"
key_location = "keys/private.pem"
```

---

## 2. Monorepo Layout & Client Packages

The repository has transitioned to a clean, language-segmented monorepo structure:

| Component | Previous Path | New Path | Bazel Target |
| :--- | :--- | :--- | :--- |
| **Go Client** | `pkg/modenv/` | `clients/go/pkg/modenv/` | `//clients/go/pkg/modenv` |
| **Universal CLI** | `cmd/modenv/` | `cmd/cli/` | `//cmd/cli` (or `//:modenv`) |
| **Python Client** | *(new)* | `clients/python/` | `//clients/python:modenv` |
| **Java Client** | *(new)* | `clients/java/` | `//clients/java:modenv` |
| **TypeScript Client** | *(new)* | `clients/typescript/` | `//clients/typescript:modenv_sources` |
| **Test Fixtures** | `test/` | `test/configs/` | `//test:test_configs` |

### Go Client Dependency Update
If you consume `modenv` via Bazel:
```starlark
# Update from:
# deps = ["@modenv//pkg/modenv"]

# To:
deps = ["@modenv//clients/go/pkg/modenv"]
```
If importing via standard Go modules, the Go module path remains unchanged:
```go
import "github.com/rrmcguinness/modenv/pkg/modenv"
```

---

## 3. Universal CLI Migration

The CLI has been unified into a single cross-platform Go binary under `cmd/cli/`:
- Bazel run target: `bazel run //cmd/cli -- <command>` (or `bazel run //:modenv -- <command>`).
- Flag parsing: `modenv encode` now supports `--type <simple|pks>`, `--public-key <path>`, and `--legacy`.
- Scaffolded templates: `modenv setup` now provisions updated templates with `simple://` syntax and commented `[secrets_store]` options.

---

## 4. Build System

The root `Makefile` has been deprecated and removed. All workflows are standardized on hermetic Bazel 8.x targets:
```bash
# Test all language clients, CLI, and docs
bazel test //...

# Build all binaries
bazel build //...
```
Language-native workflows (`go test`, `uv run pytest`, `mvn test`, `npm test`) remain fully supported within their respective package directories.
