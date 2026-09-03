---
title: "Integration Guide"
weight: 1
---

# Python Integration Guide

## Installation

### With Bazel
Add `modenv` as a dependency in your `BUILD.bazel`:
```python
load("@rules_python//python:defs.bzl", "py_library")

py_library(
    name = "my_worker",
    srcs = ["main.py"],
    deps = ["//clients/python:modenv"],
)
```

### With uv / pip
```bash
uv add modenv
# or
pip install -e clients/python/
```

## Basic Dictionary Loading

```python
from modenv import load

config = load()
print(f"App Name: {config.get('app_name')}")
database = config.get("database", {})
print(f"Database Host: {database.get('host')}")
```

## Dataclass Binding with Defensive Copying

```python
from dataclasses import dataclass, field
from modenv import load

@dataclass
class DatabaseConfig:
    host: str = "localhost"
    user: str = "root"
    password: str = ""  # Values with 'xor:' decrypt automatically

@dataclass
class AppConfig:
    app_name: str = ""
    port: int = 8080
    features: list[str] = field(default_factory=list)
    database: DatabaseConfig = field(default_factory=DatabaseConfig)

# Populate dataclass and receive an isolated clone
config = load(AppConfig())

print(f"Server {config.app_name} running on port {config.port}")
print(f"Database password decrypted: {config.database.password}")
```

## Environment Variable Lifecycle

```python
from modenv import EnvManager

manager = EnvManager()

# Track and override environment
manager.set("MODENV_RUNTIME", "production")
val, exists = manager.lookup("MODENV_RUNTIME")
print(f"Runtime: {val} (exists: {exists})")

# Revert all changes
manager.restore()
```

## Running Tests with Bazel

```bash
bazel test //clients/python/...
```
