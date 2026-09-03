---
title: "Python"
weight: 20
---

# Python Implementation

The Python client library of `modenv` is located in `clients/python/`. It uses Python 3.13 strict typing, standard library `tomllib`, and `uv` for project lifecycle management.

## Directory Layout
- `clients/python/src/modenv/`: Core library package (`modenv.py`, `py.typed`).
- `clients/python/tests/`: Unit and integration test suites using `pytest`.
- `clients/python/pyproject.toml`: Project metadata managed by `uv`.

See [Python Integration Guide](integration/) for usage instructions and code examples.
