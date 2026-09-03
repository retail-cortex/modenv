---
title: "Integration Guide"
weight: 1
---

# Go Integration Guide

## Installation

### With Bazel
Add `modenv` as a dependency in your `BUILD.bazel`:
```python
load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "my_service",
    srcs = ["main.go"],
    deps = ["//clients/go/pkg/modenv"],
)
```

### With Go Modules
```bash
go get github.com/retail-cortex/modenv/go
```

## Basic Map Loading

```go
package main

import (
	"fmt"
	"log"

	"github.com/rrmcguinness/modenv/pkg/modenv"
)

func main() {
	cfg, err := modenv.Load(nil)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	cfgMap := cfg.(map[string]interface{})
	fmt.Printf("Application: %s\n", cfgMap["app_name"])
}
```

## Struct Binding with Defensive Copying

```go
package main

import (
	"fmt"
	"log"

	"github.com/rrmcguinness/modenv/pkg/modenv"
)

type Config struct {
	AppName  string   `toml:"app_name"`
	Port     int      `toml:"port"`
	Features []string `toml:"features"`
	Database DBConfig `toml:"database"`
}

type DBConfig struct {
	Host     string `toml:"host"`
	User     string `toml:"user"`
	Password string `toml:"password"` // Encrypted secrets (xor:...) are transparently decrypted
}

func main() {
	var cfg Config
	cloneInterface, err := modenv.Load(&cfg)
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	appConfig := cloneInterface.(*Config)
	fmt.Printf("Starting %s on port %d\n", appConfig.AppName, appConfig.Port)
	fmt.Printf("Database connected to %s with decrypted password\n", appConfig.Database.Host)
}
```

## Environment Variable Lifecycle

```go
package main

import (
	"fmt"

	"github.com/rrmcguinness/modenv/pkg/modenv"
)

func main() {
	manager := modenv.New()

	manager.Set("SERVICE_TOKEN", "prod-token-xyz")
	val, exists := manager.Lookup("SERVICE_TOKEN")
	fmt.Printf("Token active: %v, Value: %s\n", exists, val)

	// Reverts SERVICE_TOKEN to its initial state
	manager.Restore()
}
```

## Running Tests with Bazel

```bash
bazel test //clients/go/...
```
