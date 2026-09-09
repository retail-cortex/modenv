---
title: "Python"
weight: 20
---

# Python Implementation

The Python client library of `modenv` is distributed as [`modenv-rc` on PyPI](https://pypi.org/project/modenv-rc/) and located in `clients/python/` within the monorepo. It uses Python 3.13 strict typing, standard library `tomllib`, and `uv` for project lifecycle management.

[![PyPI](https://img.shields.io/pypi/v/modenv-rc?color=blue&label=PyPI%3A%20modenv-rc)](https://pypi.org/project/modenv-rc/)
[![Python Versions](https://img.shields.io/pypi/pyversions/modenv-rc.svg)](https://pypi.org/project/modenv-rc/)

## Installation

```bash
pip install modenv-rc
# or
uv add modenv-rc
```

> **Import Note**: The distribution package is named `modenv-rc`, and you import it directly as `modenv`:
> ```python
> import modenv
> from modenv import load, EnvManager
> ```

## Directory Layout
- `clients/python/src/modenv/`: Core library package (`modenv.py`, `py.typed`).
- `clients/python/tests/`: Unit and integration test suites using `pytest`.
- `clients/python/pyproject.toml`: Project metadata and build configuration.

See [Python Integration Guide](integration/) for detailed usage instructions and code examples.
