# modenv for Python (`modenv-rc`)

[![PyPI version](https://img.shields.io/pypi/v/modenv-rc.svg)](https://pypi.org/project/modenv-rc/)
[![Python versions](https://img.shields.io/pypi/pyversions/modenv-rc.svg)](https://pypi.org/project/modenv-rc/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://raw.githubusercontent.com/retail-cortex/modenv/main/LICENSE)
[![Documentation](https://img.shields.io/badge/docs-retail--cortex.github.io%2Fmodenv-blue.svg)](https://retail-cortex.github.io/modenv/)

**`modenv`** is an enterprise configuration engine that simplifies application configuration by merging cascading TOML configuration layers, dynamically resolving runtime environment overlays, and transparently decrypting secrets on load.

---

## Key Features

- **Hierarchical TOML Overlays**: Automatically deep-merges base configuration (`.env.toml`), runtime environment layers (`.env.${MODENV_RUNTIME}.toml`), and local overrides (`.env.local.toml`).
- **Dataclass Binding & Defensive Copying**: Load configuration directly into typed Python `@dataclass` structures or plain `dict` objects with isolated defensive copies to prevent unintended runtime mutation.
- **Transparent Secret Decryption**: Values prefixed with URI schemes are decrypted automatically when loaded:
  - `simple://` & legacy `xor:` — Symmetric encryption using `MODENV_KEY`.
  - `pks://` — Enterprise RSA asymmetric encryption (PKCS#1 v1.5).
  - `cloud://` — Direct Google Cloud Secret Manager resolution.
- **Environment Lifecycle Management**: `EnvManager` tracks modifications to environment variables and provides deterministic rollbacks.
- **Universal CLI Companion**: Works alongside the standalone `modenv` CLI to scaffold configurations, inspect merged configurations, and encrypt secrets across teams.

---

## Installation

Install via `pip`:
```bash
pip install modenv-rc
```

Or using `uv`:
```bash
uv add modenv-rc
```

*(Note: The import path is `import modenv`)*

---

## Quickstart Guide

### 1. Configure Your TOML Files

`modenv` loads and merges configuration files based on the directory specified by `MODENV_PREFIX` (defaults to current working directory):

```
my-app/
├── .env.toml                 # Base configuration (required)
├── .env.production.toml      # Runtime overlay (e.g. MODENV_RUNTIME=production)
└── .env.local.toml           # Developer local override (git-ignored)
```

#### `.env.toml` (Base)
```toml
app_name = "MyService"
port = 8080

[database]
host = "localhost"
port = 5432
user = "app_user"
password = "simple://0a1b2c3d4e..."  # Encrypted secret
```

#### `.env.production.toml` (Production Overlay)
```toml
port = 443

[database]
host = "prod-db.internal"
password = "pks://grKhOtE7TXQ..."    # Asymmetric RSA secret or cloud:// secret
```

---

### 2. Loading with Dataclasses (Recommended)

Bind configuration directly to strongly-typed Python `@dataclass` structures. Secrets are decrypted on the fly:

```python
from dataclasses import dataclass, field
from modenv import load

@dataclass
class DatabaseConfig:
    host: str = "localhost"
    port: int = 5432
    user: str = "root"
    password: str = ""  # Decrypted transparently (simple://, pks://, or cloud://)

@dataclass
class AppConfig:
    app_name: str = ""
    port: int = 8080
    database: DatabaseConfig = field(default_factory=DatabaseConfig)

# Automatically discovers .env.toml and active overlays
config = load(AppConfig())

print(f"Service '{config.app_name}' running on port {config.port}")
print(f"Connecting to database at {config.database.host}:{config.database.port}")
print(f"Decrypted password: {config.database.password}")
```

---

### 3. Loading as a Dictionary

You can also load raw, deep-merged configuration as a standard Python dictionary:

```python
from modenv import load

config = load()

app_name = config.get("app_name")
db_host = config.get("database", {}).get("host")
db_password = config.get("database", {}).get("password")
```

---

### 4. Managing Environment Variables (`EnvManager`)

Safely manage environment variable mutations and rollback changes when finished (ideal for tests and worker processes):

```python
from modenv import EnvManager

manager = EnvManager()

# Modify environment while recording initial state
manager.set("MODENV_RUNTIME", "staging")

val, exists = manager.lookup("MODENV_RUNTIME")
print(f"Active Runtime: {val}")

# Restore all modified variables to their original values
manager.restore()
```

---

## Universal CLI (`modenv`)

`modenv` provides a hermetic, single-binary CLI written in Go to scaffold configuration templates, inspect merged configuration trees, and encrypt secrets.

### Downloading the Precompiled CLI

Precompiled binaries for Linux, macOS, and Windows are available for every release on [GitHub Releases](https://github.com/retail-cortex/modenv/releases):

| OS / Platform | Architecture | Binary |
| :--- | :--- | :--- |
| **Linux** | AMD64 (x86_64) | `modenv-linux-amd64.tar.gz` |
| **Linux** | ARM64 (aarch64) | `modenv-linux-arm64.tar.gz` |
| **macOS (Darwin)** | Apple Silicon (ARM64) | `modenv-darwin-arm64.tar.gz` |
| **macOS (Darwin)** | Intel (x86_64) | `modenv-darwin-amd64.tar.gz` |
| **Windows** | AMD64 (x86_64) | `modenv-windows-amd64.zip` |

#### Quick Install (macOS / Linux):
```bash
# Download and extract the latest binary for your system:
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/' -e 's/arm64/arm64/')
curl -sSL "https://github.com/retail-cortex/modenv/releases/latest/download/modenv-${OS}-${ARCH}.tar.gz" | tar -xz

# Move to your path
chmod +x modenv*
sudo mv modenv* /usr/local/bin/modenv
```

---

### Key CLI Commands

#### 1. Scaffold Configuration Files (`setup`)
Generates `.env.toml` and `.env.local.toml` templates in the current directory:
```bash
modenv setup
```

#### 2. Encrypt Secrets (`encode`)
Encrypt sensitive strings before committing them to TOML files:

```bash
# Encrypt with symmetric key (MODENV_KEY)
modenv encode "my-database-password"
# Output: simple://01000704...

# Encrypt with asymmetric RSA public key
modenv encode --type=pks --public-key=keys/public.pem "my-database-password"
# Output: pks://grKhOtE7TXQNSt3bht5iatVOf...
```

#### 3. Inspect Merged Configuration (`read`)
Merges overlays, decrypts secrets, and prints the resolved configuration tree:
```bash
# Inspect development configuration
modenv read

# Inspect production configuration
MODENV_RUNTIME=production modenv read
```

---

## Environment Variables

| Variable | Default | Description |
| :--- | :--- | :--- |
| `MODENV_PREFIX` | `.` (CWD) | Base directory path where `.env.*.toml` files and keys reside. |
| `MODENV_RUNTIME` | *(empty)* | Active environment name (e.g. `production`). Loads `.env.${MODENV_RUNTIME}.toml`. |
| `MODENV_KEY` | `modenv-default-key` | Symmetric key used to encrypt/decrypt `simple://` and `xor:` tokens. |
| `MODENV_PRIVATE_KEY` | *(empty)* | PEM string of the RSA private key used to decrypt `pks://` tokens. |
| `MODENV_KEY_LOCATION` | *(empty)* | File path to the RSA private key PEM file for `pks://` tokens. |
| `GOOGLE_CLOUD_PROJECT`| *(empty)* | GCP project ID used for `cloud://` Secret Manager lookups. |

---

## Documentation & Links

- **Documentation Site**: [https://retail-cortex.github.io/modenv/](https://retail-cortex.github.io/modenv/)
- **Python Integration Guide**: [https://retail-cortex.github.io/modenv/docs/python/integration/](https://retail-cortex.github.io/modenv/docs/python/integration/)
- **GitHub Repository**: [https://github.com/retail-cortex/modenv](https://github.com/retail-cortex/modenv)
- **Issue Tracker**: [https://github.com/retail-cortex/modenv/issues](https://github.com/retail-cortex/modenv/issues)
- **Releases**: [https://github.com/retail-cortex/modenv/releases](https://github.com/retail-cortex/modenv/releases)

---

## License

Licensed under the [Apache License, Version 2.0](https://raw.githubusercontent.com/retail-cortex/modenv/main/LICENSE).
