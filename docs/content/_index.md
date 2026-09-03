---
title: "modenv"
type: docs
---

# modenv: Polyglot Configuration & Secret Loader

`modenv` provides hierarchical, cascading TOML configuration loading, in-place secret decryption, and environment state management across Go, Python, Java, and TypeScript.

All language implementations adhere to a unified set of architectural tenets, tested and built hermetically using Bazel.

```mermaid
graph TD
    A[".env.toml (Base, Required)"] --> D["Deep Merge Engine"]
    B[".env.${MODENV_RUNTIME}.toml (Optional)"] --> D
    C[".env.local.toml (Local Override)"] --> D
    D --> E["In-Place XOR Decryption"]
    E --> F["Defensive Copy / Type Binding"]
    F --> G["Go Struct / Python Dataclass / Java POJO / TS Object"]
```

## Documentation & Guides

### Core Concepts
- [Tenets of Configuration]({{< relref "/docs" >}}): The 7 foundational rules of modenv.
- [Purpose & Methodology]({{< relref "/docs/purpose" >}}): Why modenv exists, pain points solved, and core design principles.
- [Architecture & Dataflow]({{< relref "/docs/architecture" >}}): Complete end-to-end dataflow pipeline, sequence diagrams, and subsystems.
- [Configuration Hierarchy & Merging]({{< relref "/docs/hierarchy" >}}): Cascading precedence matrix, deep merge rules, and prefix path routing.
- [Universal CLI]({{< relref "/docs/cli" >}}): Cross-platform Go binary, commands reference, and multi-architecture release targets.

### Language Integration Guides
- [Go Integration]({{< relref "/docs/go/integration" >}}): Struct binding, library usage, and test fixtures.
- [Python Integration]({{< relref "/docs/python/integration" >}}): Dataclass mapping, uv setup, and typed access.
- [Java Integration]({{< relref "/docs/java/integration" >}}): POJO binding, Maven artifacts, and reflection mapping.
- [TypeScript Integration]({{< relref "/docs/typescript/integration" >}}): ESM interface mapping, pnpm packages, and zero-dependency runtime.
