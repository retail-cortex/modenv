# modenv for Java

Hierarchical TOML configuration, secret decryption, and environment loader for Java.

Part of the [modenv](https://github.com/retail-cortex/modenv) multi-language ecosystem.

## Quickstart

```java
import com.retailcortex.modenv.Modenv;

public class Main {
    public static class AppConfig {
        public String appName;
        public int port;
        public DatabaseConfig database = new DatabaseConfig();
    }
    public static class DatabaseConfig {
        public String password; // Encrypted "xor:..." values decrypt automatically
    }

    public static void main(String[] args) throws Exception {
        AppConfig config = Modenv.load(new AppConfig());
        System.out.printf("Started %s on port %d%n", config.appName, config.port);
    }
}
```

## License

Apache-2.0
