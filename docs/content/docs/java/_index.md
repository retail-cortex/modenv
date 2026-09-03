---
title: "Java"
weight: 30
---

# Java Implementation

The Java client library of `modenv` is located in `clients/java/`. It conforms to standard Maven layout (`src/main/java`, `src/test/java`), integrates with `org.tomlj` for TOML compliance, and leverages JUnit 5 for testing.

## Directory Layout
- `clients/java/src/main/java/com/retailcortex/modenv/`:
  - `Modenv.java`: Core configuration loader and cryptographic routines.
  - `EnvManager.java`: Thread-safe environment tracking and overlay manager.
- `clients/java/src/test/java/com/retailcortex/modenv/`: JUnit 5 test suites.
- `clients/java/pom.xml`: Standard Maven project descriptor.

See [Java Integration Guide](integration/) for usage instructions and code examples.
