---
title: "Go"
weight: 10
---

# Go Implementation

The Go client library of `modenv` is located in `clients/go/`. It provides zero-allocation parsing bindings, deep struct reflection, and full Bazel integration via `rules_go`.

## Directory Layout
- `clients/go/pkg/modenv/`: Core library package (`modenv.go`).
- `clients/go/test/`: Integration tests and test fixtures.
- `cmd/cli/`: Universal cross-platform command-line executable (`main.go`).

See [Go Integration Guide](integration/) for usage instructions and code examples.
