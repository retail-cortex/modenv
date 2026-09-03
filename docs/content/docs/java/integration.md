---
title: "Integration Guide"
weight: 1
---

# Java Integration Guide

## Installation

### With Bazel
Add `modenv` as a dependency in your `BUILD.bazel`:
```python
load("@rules_java//java:defs.bzl", "java_library")

java_library(
    name = "my_service",
    srcs = ["MyService.java"],
    deps = ["//clients/java:modenv"],
)
```

### With Maven (`pom.xml`)
```xml
<dependency>
    <groupId>com.retailcortex</groupId>
    <artifactId>modenv</artifactId>
    <version>0.0.1</version>
</dependency>
```

## Basic Map Loading

```java
package com.mycompany.app;

import com.retailcortex.modenv.Modenv;
import java.io.IOException;
import java.util.Map;

public class Main {
    public static void main(String[] args) throws IOException {
        Map<String, Object> config = Modenv.load();
        System.out.println("App Name: " + config.get("app_name"));

        @SuppressWarnings("unchecked")
        Map<String, Object> db = (Map<String, Object>) config.get("database");
        System.out.println("DB Host: " + db.get("host"));
    }
}
```

## POJO Binding with Defensive Copying

```java
package com.mycompany.app;

import com.retailcortex.modenv.Modenv;
import java.io.IOException;
import java.util.List;

public class Main {
    public static class AppConfig {
        public String appName;
        public int port;
        public List<String> features;
        public DatabaseConfig database = new DatabaseConfig();
    }

    public static class DatabaseConfig {
        public String host;
        public String user;
        public String password; // Values starting with xor: are decrypted automatically
    }

    public static void main(String[] args) throws IOException {
        AppConfig config = Modenv.load(new AppConfig());

        System.out.printf("Starting %s on port %d%n", config.appName, config.port);
        System.out.printf("Connecting to database at %s%n", config.database.host);
    }
}
```

## Environment Variable Lifecycle

```java
package com.mycompany.app;

import com.retailcortex.modenv.EnvManager;

public class Main {
    public static void main(String[] args) {
        EnvManager manager = EnvManager.getInstance();

        manager.set("MODENV_RUNTIME", "production");
        EnvManager.LookupResult lookup = manager.lookup("MODENV_RUNTIME");
        System.out.println("Runtime: " + lookup.getValue() + " (exists: " + lookup.exists() + ")");

        // Revert all mutated variables
        manager.restore();
    }
}
```

## Running Tests with Bazel

```bash
bazel test //clients/java/...
```
