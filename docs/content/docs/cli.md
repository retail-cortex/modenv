---
title: "Universal CLI"
weight: 6
---

# Universal CLI (`modenv`)

`modenv` ships a single, hermetic, cross-platform command-line executable written in Go located at [`cmd/cli/`](file:///Users/rmcguinness/Projects/retail-cortex/modenv/cmd/cli).

Following the DRY (Don't Repeat Yourself) principle, language-specific client libraries ([`clients/go`](file:///Users/rmcguinness/Projects/retail-cortex/modenv/clients/go), [`clients/java`](file:///Users/rmcguinness/Projects/retail-cortex/modenv/clients/java), [`clients/python`](file:///Users/rmcguinness/Projects/retail-cortex/modenv/clients/python), [`clients/typescript`](file:///Users/rmcguinness/Projects/retail-cortex/modenv/clients/typescript)) are pure embedding packages without redundant CLI wrappers. The universal Go CLI serves all developers, scripts, and deployment pipelines across every platform.

---

## 1. Commands Reference

### `setup`
Scaffolds standard configuration files in the directory specified by `MODENV_PREFIX` (or current working directory) if they do not already exist:
- `.env.toml`: Base schema template including `[secrets_store]` default block.
- `.env.local.toml`: Local developer override template.

```bash
modenv setup
```

### `read`
Inspects, deep-merges, decrypts all smart secrets (`simple://`, legacy `xor:`, `pks://`, and `cloud://`), and prints the resolved configuration tree as valid TOML for the currently active runtime environment (`MODENV_RUNTIME`).

```bash
# Read resolved configuration for development
modenv read

# Read resolved configuration for production
MODENV_RUNTIME=production modenv read
```

### `encode [options] <value>`
Encrypts a sensitive plaintext string into a smart secret URI token that can be safely committed to `.env.toml` or runtime overlays.

#### Options:
- `-t, --type <simple|pks>`: Secret encryption algorithm. Defaults to `simple`.
- `-k, --public-key <path>`: File path to the RSA public key PEM file (required when `--type=pks`).
- `--legacy`: Format simple XOR secret using the legacy `xor:` prefix instead of `simple://`.

```bash
# Simple XOR secret (simple://...)
modenv encode "production-db-password"
# Output: simple://01000704...

# Legacy XOR prefix (xor:...)
modenv encode --legacy "production-db-password"
# Output: xor:01000704...

# Asymmetric RSA Public Key Secret (pks://...)
modenv encode --type=pks --public-key=keys/app_public.pem "production-db-password"
# Output: pks://grKhOtE7TXQNSt3bht5iatVOf...
```

---

## 2. Environment Variables

| Variable | Default | Description |
| :--- | :--- | :--- |
| `MODENV_PREFIX` | `.` (CWD) | Base directory path where `.env.*.toml` files and relative keys are located. |
| `MODENV_RUNTIME` | *(empty)* | Active environment name (e.g. `production`, `staging`). Loads `.env.${MODENV_RUNTIME}.toml`. |
| `MODENV_KEY` | `modenv-default-key` | Symmetric key used to encrypt and decrypt `simple://` and `xor:` tokens. |
| `MODENV_PRIVATE_KEY` | *(empty)* | PEM string of the RSA private key used to decrypt `pks://` tokens. |
| `MODENV_KEY_LOCATION` | *(empty)* | Path to the RSA private key PEM file used to decrypt `pks://` tokens. |
| `GOOGLE_CLOUD_PROJECT` | *(empty)* | Google Cloud Project ID used for `cloud://` Secret Manager lookups. |
| `GOOGLE_OAUTH_ACCESS_TOKEN` | *(empty)* | OAuth2 access token for Google Cloud Secret Manager REST requests. |

---

## 3. Building and Running with Bazel

### Execute via Bazel Run
```bash
# Run CLI help
bazel run //cmd/cli -- help

# Run through root workspace alias
bazel run //:modenv -- help

# Encode secret token
bazel run //cmd/cli -- encode "secret_token"

# Inspect resolved configuration tree
bazel run //cmd/cli -- read
```

### Build Statically Linked Host Binary
```bash
bazel build //cmd/cli

# Binary output:
./bazel-bin/cmd/cli/cli_/cli --help
```

---

## 4. Cross-Platform Binary Distributions

The release workflow cross-compiles statically linked standalone binaries with zero external runtime dependencies for all major platforms:

| Platform | Architecture | Binary Artifact | Archive Format |
| :--- | :--- | :--- | :--- |
| **Linux** | AMD64 (x86_64) | `modenv-linux-amd64` | `modenv-linux-amd64.tar.gz` |
| **Linux** | ARM64 (aarch64) | `modenv-linux-arm64` | `modenv-linux-arm64.tar.gz` |
| **macOS (Darwin)** | Apple Silicon (ARM64) | `modenv-darwin-arm64` | `modenv-darwin-arm64.tar.gz` |
| **macOS (Darwin)** | Intel (x86_64) | `modenv-darwin-amd64` | `modenv-darwin-amd64.tar.gz` |
| **Windows** | AMD64 (x86_64) | `modenv-windows-amd64.exe` | `modenv-windows-amd64.zip` |

### Cross-Compiling with Bazel
```bash
# Linux AMD64
bazel build --platforms=@rules_go//go/toolchain:linux_amd64 //cmd/cli

# Linux ARM64
bazel build --platforms=@rules_go//go/toolchain:linux_arm64 //cmd/cli

# macOS Apple Silicon
bazel build --platforms=@rules_go//go/toolchain:darwin_arm64 //cmd/cli

# macOS Intel
bazel build --platforms=@rules_go//go/toolchain:darwin_amd64 //cmd/cli

# Windows AMD64
bazel build --platforms=@rules_go//go/toolchain:windows_amd64 //cmd/cli
```
All release bundles include automated SHA256 checksum manifests (`checksums.txt`) verified during continuous integration.
