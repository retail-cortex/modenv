# Test Suite Setup

This directory contains the integration tests and configuration fixtures for testing `modenv`.

## Structure

- **`integration_test.go`**: Core integration test cases verifying full package loading, environment variable management, and local override execution flow.
- **`configs/`**: Static configuration directories containing TOML fixtures mapped to test cases:
  - `default`: Base configuration loading assertions.
  - `defensive`: Verifies memory safety and copy validation.
  - `integration`: Standard integration environment values.
  - `local-override`: Validation of runtime overrides combined with local override priority.
  - `missing`: Empty directory to check error behavior on missing base files.
  - `prefix-test`: Validates the custom `MODENV_PREFIX` location logic.
  - `runtime`: Validates `MODENV_RUNTIME` specific merging.
  - `secrets`: Contains encrypted properties (using `xor:`) to assert transparent decryption.
  - `struct`: Asserts type binding verification.

## Configuration Paths Resolution

Unit and integration tests dynamically find this directory by crawling up the current working directory path to resolve absolute file references. 

This mechanism allows executing tests from:
1. The repository root (`go test ./...` or `make test`)
2. Individual package subdirectories (`go test ./pkg/modenv`)
3. The Bazel sandbox environment (`bazel test //...`)

## Bazel Integration

To allow tests running inside the Bazel sandbox to access these static TOML config fixtures, we register a `filegroup` target:

```starlark
filegroup(
    name = "test_configs",
    srcs = glob(["configs/**"]),
    visibility = ["//visibility:public"],
)
```

The test targets declare a dependency on this filegroup using the `data` attribute:

```starlark
go_test(
    name = "integration_test",
    srcs = ["integration_test.go"],
    data = [":test_configs"],
    deps = [
        "//pkg/modenv",
        "@com_github_stretchr_testify//assert",
    ],
)
```
