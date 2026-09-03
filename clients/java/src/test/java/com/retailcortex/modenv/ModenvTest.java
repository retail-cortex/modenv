/*
 * Copyright 2026 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package com.retailcortex.modenv;

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.io.FileNotFoundException;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.Base64;
import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;

public class ModenvTest {

    public static class AppConfig {
        public String appName;
        public int port;
        public List<String> features;
        public DBConfig database = new DBConfig();
    }

    public static class DBConfig {
        public String host;
        public String user;
        public String password;
    }

    @BeforeEach
    public void setUp() {
        EnvManager.getInstance().restore();
    }

    @AfterEach
    public void tearDown() {
        EnvManager.getInstance().restore();
    }

    private static Path getTestConfigsDir() {
        Path curr = Paths.get("").toAbsolutePath();
        while (curr != null) {
            Path candidate = curr.resolve("test/configs");
            if (Files.isDirectory(candidate)) {
                return candidate;
            }
            curr = curr.getParent();
        }
        return Paths.get("test/configs").toAbsolutePath();
    }

    @Test
    public void testSecretEncryptionDecryptionRoundtrip() {
        String plain = "my-super-secret-password-123!";

        // Default key (simple://)
        String encoded = Modenv.encryptSecret(plain);
        assertTrue(encoded.startsWith("simple://"));
        String decoded = Modenv.decryptSecret(encoded);
        assertEquals(plain, decoded);

        // Legacy key (xor:)
        String encodedLegacy = Modenv.encryptLegacySecret(plain);
        assertTrue(encodedLegacy.startsWith("xor:"));
        String decodedLegacy = Modenv.decryptSecret(encodedLegacy);
        assertEquals(plain, decodedLegacy);

        // Custom key
        EnvManager.getInstance().set("MODENV_KEY", "custom-super-long-key-spec");
        String encodedCustom = Modenv.encryptSecret(plain);
        assertNotEquals(encoded, encodedCustom);
        String decodedCustom = Modenv.decryptSecret(encodedCustom);
        assertEquals(plain, decodedCustom);
    }

    @Test
    public void testInvalidSecretDecryption() {
        assertThrows(IllegalArgumentException.class, () -> Modenv.decryptSecret(null));
        assertThrows(IllegalArgumentException.class, () -> Modenv.decryptSecret("plain-not-prefixed"));
        assertThrows(IllegalArgumentException.class, () -> Modenv.decryptSecret("simple://invalid-non-hex-zz"));
        assertThrows(IllegalArgumentException.class, () -> Modenv.decryptSecret("xor:invalid-non-hex-zz"));
        assertThrows(IllegalArgumentException.class, () -> Modenv.decryptSecret("xor:123")); // odd length
    }

    @Test
    @SuppressWarnings("unchecked")
    public void testLoadWithEncryptedSecrets() throws IOException {
        Path dir = getTestConfigsDir().resolve("secrets");
        EnvManager.getInstance().set("MODENV_PREFIX", dir.toString());

        Map<String, Object> cfg = Modenv.load();
        assertNotNull(cfg);
        assertEquals("secret-app", cfg.get("app_name"));

        Map<String, Object> db = (Map<String, Object>) cfg.get("database");
        assertNotNull(db);
        assertEquals("local_db_password", db.get("password"));
    }

    @Test
    public void testLoadWithPrefix() throws IOException {
        Path dir = getTestConfigsDir().resolve("prefix-test");
        EnvManager.getInstance().set("MODENV_PREFIX", dir.toString());

        Map<String, Object> cfg = Modenv.load();
        assertEquals("prefixed-app", cfg.get("app_name"));
        assertEquals(2000L, ((Number) cfg.get("port")).longValue());
    }

    @Test
    @SuppressWarnings("unchecked")
    public void testLoadMapDefault() throws IOException {
        Path dir = getTestConfigsDir().resolve("default");
        EnvManager.getInstance().set("MODENV_PREFIX", dir.toString());

        Map<String, Object> cfg = Modenv.load();
        assertEquals("modenv-default", cfg.get("app_name"));
        assertEquals(8080L, ((Number) cfg.get("port")).longValue());

        Map<String, Object> db = (Map<String, Object>) cfg.get("database");
        assertEquals("localhost", db.get("host"));
    }

    @Test
    public void testLoadStruct() throws IOException {
        Path dir = getTestConfigsDir().resolve("struct");
        EnvManager.getInstance().set("MODENV_PREFIX", dir.toString());

        AppConfig cfg = new AppConfig();
        AppConfig clone = Modenv.load(cfg);

        assertEquals("modenv-struct", cfg.appName);
        assertEquals(9000, cfg.port);
        assertEquals(List.of("web"), cfg.features);
        assertEquals("db", cfg.database.host);

        assertEquals("modenv-struct", clone.appName);
        assertEquals(9000, clone.port);
    }

    @Test
    @SuppressWarnings("unchecked")
    public void testLoadRuntimeOverride() throws IOException {
        Path dir = getTestConfigsDir().resolve("runtime");
        EnvManager.getInstance().set("MODENV_PREFIX", dir.toString());
        EnvManager.getInstance().set("MODENV_RUNTIME", "production");

        Map<String, Object> cfg = Modenv.load();
        assertEquals("base", cfg.get("app_name"));
        assertEquals(9999L, ((Number) cfg.get("port")).longValue());

        Map<String, Object> db = (Map<String, Object>) cfg.get("database");
        assertEquals("prod-db", db.get("host"));
    }

    @Test
    @SuppressWarnings("unchecked")
    public void testLoadLocalOverrideLast() throws IOException {
        Path dir = getTestConfigsDir().resolve("local-override");
        EnvManager.getInstance().set("MODENV_PREFIX", dir.toString());
        EnvManager.getInstance().set("MODENV_RUNTIME", "production");

        Map<String, Object> cfg = Modenv.load();
        assertEquals("prod", cfg.get("app_name"));
        assertEquals(7777L, ((Number) cfg.get("port")).longValue());

        Map<String, Object> db = (Map<String, Object>) cfg.get("database");
        assertEquals("local-db", db.get("host"));
    }

    @Test
    public void testLoadDefensiveCopy() throws IOException {
        Path dir = getTestConfigsDir().resolve("defensive");
        EnvManager.getInstance().set("MODENV_PREFIX", dir.toString());

        AppConfig target = new AppConfig();
        AppConfig clone = Modenv.load(target);

        assertEquals("defensive", target.appName);
        assertEquals("defensive", clone.appName);

        clone.appName = "mutated";
        clone.features.set(0, "mutated-feature");
        clone.database.host = "mutated-host";

        assertEquals("defensive", target.appName);
        assertEquals("original", target.features.get(0));
        assertEquals("original-host", target.database.host);
    }

    @Test
    public void testLoadMissingBaseFileThrows() {
        Path dir = getTestConfigsDir().resolve("missing");
        EnvManager.getInstance().set("MODENV_PREFIX", dir.toString());

        assertThrows(FileNotFoundException.class, Modenv::load);
    }

    @Test
    public void testEnvManager() {
        EnvManager m = new EnvManager();
        m.set("TEST_NEW_KEY", "new_val");

        EnvManager.LookupResult res = m.lookup("TEST_NEW_KEY");
        assertTrue(res.exists());
        assertEquals("new_val", res.getValue());

        m.set("TEST_NEW_KEY", "modified_val");
        assertEquals("modified_val", m.get("TEST_NEW_KEY"));

        m.unset("TEST_NEW_KEY");
        assertFalse(m.lookup("TEST_NEW_KEY").exists());

        m.restore();
        assertFalse(m.lookup("TEST_NEW_KEY").exists());
    }

    public static class ComplexConfig {
        public String appName;
        public long port;
        public double weight;
        public boolean enabled;
        public DBConfig database;
        public List<String> features;
    }

    @Test
    public void testLoadWithMapTarget() throws IOException {
        Path dir = getTestConfigsDir().resolve("default");
        EnvManager.getInstance().set("MODENV_PREFIX", dir.toString());

        Map<String, Object> targetMap = new java.util.LinkedHashMap<>();
        Map<String, Object> clone = Modenv.load(targetMap);
        assertNotNull(clone);
        assertEquals("modenv-default", targetMap.get("app_name"));
    }

    @Test
    public void testLoadComplexConfigNullFields() throws IOException {
        Path tmpDir = Files.createTempDirectory("modenv-complex-test");
        String toml = "app_name = \"complex\"\n"
                + "port = 5000\n"
                + "weight = 12.5\n"
                + "enabled = true\n"
                + "features = [\"a\", \"b\"]\n"
                + "[database]\n"
                + "host = \"remote\"\n";
        Files.write(tmpDir.resolve(".env.toml"), toml.getBytes(java.nio.charset.StandardCharsets.UTF_8));
        EnvManager.getInstance().set("MODENV_PREFIX", tmpDir.toString());

        ComplexConfig cfg = new ComplexConfig();
        ComplexConfig clone = Modenv.load(cfg);
        assertEquals("complex", cfg.appName);
        assertEquals(5000L, cfg.port);
        assertEquals(12.5, cfg.weight, 0.001);
        assertTrue(cfg.enabled);
        assertNotNull(cfg.database);
        assertEquals("remote", cfg.database.host);
        assertNotNull(clone.database);
    }

    @Test
    public void testLoadListsWithSecretsAndNestedArrays() throws IOException {
        Path tmpDir = Files.createTempDirectory("modenv-arrays-test");
        String secret1 = Modenv.encryptSecret("nested-secret-1");
        String secret2 = Modenv.encryptSecret("nested-secret-2");
        String toml = "app_name = \"array-test\"\n"
                + "secret_list = [\"" + secret1 + "\", \"plain\"]\n"
                + "nested_arrays = [[\"" + secret2 + "\"]]\n"
                + "[[tables]]\n"
                + "sec = \"" + secret1 + "\"\n";
        Files.write(tmpDir.resolve(".env.toml"), toml.getBytes(java.nio.charset.StandardCharsets.UTF_8));
        EnvManager.getInstance().set("MODENV_PREFIX", tmpDir.toString());

        Map<String, Object> cfg = Modenv.load();
        List<?> secretList = (List<?>) cfg.get("secret_list");
        assertEquals("nested-secret-1", secretList.get(0));
        assertEquals("plain", secretList.get(1));

        List<?> nestedArrays = (List<?>) cfg.get("nested_arrays");
        List<?> innerList = (List<?>) nestedArrays.get(0);
        assertEquals("nested-secret-2", innerList.get(0));

        List<?> tables = (List<?>) cfg.get("tables");
        Map<?, ?> table1 = (Map<?, ?>) tables.get(0);
        assertEquals("nested-secret-1", table1.get("sec"));
    }

    @Test
    public void testLoadTomlSyntaxError() throws IOException {
        Path tmpDir = Files.createTempDirectory("modenv-err-test");
        Files.write(tmpDir.resolve(".env.toml"), "invalid = [ toml syntax".getBytes(java.nio.charset.StandardCharsets.UTF_8));
        EnvManager.getInstance().set("MODENV_PREFIX", tmpDir.toString());

        assertThrows(IOException.class, Modenv::load);
    }

    @Test
    public void testPKSSecretEncryptionDecryptionRoundtrip() throws IOException {
        Path configsDir = getTestConfigsDir();
        String pubPem = Files.readString(configsDir.resolve("test_rsa_public.pem"));
        String privPem = Files.readString(configsDir.resolve("test_rsa_private.pem"));

        String secret = "java-pks-secret-test";
        String encrypted = Modenv.encryptPKSSecret(secret, pubPem);
        assertTrue(encrypted.startsWith("pks://"));

        // Decrypt with MODENV_PRIVATE_KEY
        EnvManager.getInstance().set("MODENV_PRIVATE_KEY", privPem);
        String decrypted = Modenv.decryptPKSSecret(encrypted);
        assertEquals(secret, decrypted);
        EnvManager.getInstance().unset("MODENV_PRIVATE_KEY");

        // Decrypt with MODENV_KEY_LOCATION
        EnvManager.getInstance().set("MODENV_KEY_LOCATION", configsDir.resolve("test_rsa_private.pem").toString());
        decrypted = Modenv.decryptPKSSecret(encrypted);
        assertEquals(secret, decrypted);
        EnvManager.getInstance().unset("MODENV_KEY_LOCATION");

        // Decrypt with SecretsStoreConfig
        Modenv.SecretsStoreConfig cfg = new Modenv.SecretsStoreConfig("pks", "proj", "reg", configsDir.resolve("test_rsa_private.pem").toString());
        decrypted = Modenv.decryptPKSSecret(encrypted, cfg);
        assertEquals(secret, decrypted);
    }

    @Test
    public void testPKSErrors() throws IOException {
        Path configsDir = getTestConfigsDir();
        String pubPem = Files.readString(configsDir.resolve("test_rsa_public.pem"));

        assertThrows(IllegalArgumentException.class, () -> Modenv.decryptPKSSecret("xor:invalid"));
        assertThrows(IllegalStateException.class, () -> Modenv.decryptPKSSecret("pks://aaaa", new Modenv.SecretsStoreConfig()));
        assertThrows(RuntimeException.class, () -> Modenv.decryptPKSSecret("pks://aaaa", new Modenv.SecretsStoreConfig("pks", "", "", "/no/such/key.pem")));

        EnvManager.getInstance().set("MODENV_PRIVATE_KEY", "not-a-valid-pem");
        assertThrows(RuntimeException.class, () -> Modenv.decryptPKSSecret("pks://aaaa"));
        EnvManager.getInstance().unset("MODENV_PRIVATE_KEY");

        assertThrows(RuntimeException.class, () -> Modenv.encryptPKSSecret("test", "bad-pub-pem"));
    }

    @Test
    public void testCloudSecretWithEnvOverride() {
        EnvManager.getInstance().set("MODENV_CLOUD_SECRET_DB_PASS", "java-cloud-pass");
        Modenv.SecretsStoreConfig cfg = new Modenv.SecretsStoreConfig("cloud", "my-project", "us-central1", "");

        String val = Modenv.resolveCloudSecret("cloud://db-pass", cfg);
        assertEquals("java-cloud-pass", val);

        EnvManager.getInstance().set("MODENV_CLOUD_SECRET_FULL_SECRET", "full-cloud-pass");
        String val2 = Modenv.resolveCloudSecret("cloud://projects/my-project/secrets/full-secret/versions/1", cfg);
        assertEquals("full-cloud-pass", val2);
    }

    @Test
    public void testCloudSecretWithCustomResolver() {
        Modenv.setCloudSecretResolver((uri, cfg) -> "custom-" + uri);
        Modenv.SecretsStoreConfig cfg = new Modenv.SecretsStoreConfig("cloud", "my-project", "us-central1", "");
        String val = Modenv.resolveCloudSecret("cloud://custom-test", cfg);
        assertEquals("custom-cloud://custom-test", val);
        Modenv.setCloudSecretResolver(null);
    }

    @Test
    public void testCloudSecretWithMockServer() throws Exception {
        com.sun.net.httpserver.HttpServer server = com.sun.net.httpserver.HttpServer.create(new java.net.InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/", exchange -> {
            String path = exchange.getRequestURI().getPath();
            if (path.contains("db-pass")) {
                String payload = "{\"payload\":{\"data\":\"" + java.util.Base64.getEncoder().encodeToString("mock-server-secret".getBytes(java.nio.charset.StandardCharsets.UTF_8)) + "\"}}";
                exchange.getResponseHeaders().set("Content-Type", "application/json");
                exchange.sendResponseHeaders(200, payload.length());
                try (java.io.OutputStream os = exchange.getResponseBody()) {
                    os.write(payload.getBytes(java.nio.charset.StandardCharsets.UTF_8));
                }
            } else {
                exchange.sendResponseHeaders(404, 0);
                exchange.close();
            }
        });
        server.start();

        try {
            int port = server.getAddress().getPort();
            EnvManager.getInstance().set("MODENV_GCP_ENDPOINT", "http://127.0.0.1:" + port);
            EnvManager.getInstance().set("GOOGLE_OAUTH_ACCESS_TOKEN", "mock-token");
            Modenv.SecretsStoreConfig cfg = new Modenv.SecretsStoreConfig("cloud", "mock-proj", "", "");

            String val = Modenv.resolveCloudSecret("cloud://db-pass", cfg);
            assertEquals("mock-server-secret", val);

            assertThrows(RuntimeException.class, () -> Modenv.resolveCloudSecret("cloud://not-found", cfg));
        } finally {
            server.stop(0);
        }
    }

    @Test
    public void testCloudSecretErrors() {
        assertThrows(IllegalArgumentException.class, () -> Modenv.resolveCloudSecret(null, new Modenv.SecretsStoreConfig()));
        assertThrows(IllegalArgumentException.class, () -> Modenv.resolveCloudSecret("invalid://uri", new Modenv.SecretsStoreConfig()));

        EnvManager.getInstance().unset("GOOGLE_CLOUD_PROJECT");
        EnvManager.getInstance().unset("GCP_PROJECT");
        assertThrows(IllegalArgumentException.class, () -> Modenv.resolveCloudSecret("cloud://secret", new Modenv.SecretsStoreConfig()));

        Modenv.SecretsStoreConfig cfg = new Modenv.SecretsStoreConfig("cloud", "proj", "", "");
        EnvManager.getInstance().unset("MODENV_CLOUD_SECRET_UNCONFIGURED");
        EnvManager.getInstance().unset("GOOGLE_OAUTH_ACCESS_TOKEN");
        EnvManager.getInstance().unset("GCP_ACCESS_TOKEN");
        assertThrows(RuntimeException.class, () -> Modenv.resolveCloudSecret("cloud://unconfigured", cfg));
    }

    @Test
    @SuppressWarnings("unchecked")
    public void testLoadSmartSecretsDirectory() throws IOException {
        Path configsDir = getTestConfigsDir();
        Path smartDir = configsDir.resolve("smart-secrets");
        Path privKeyPath = configsDir.resolve("test_rsa_private.pem");

        EnvManager.getInstance().set("MODENV_PREFIX", smartDir.toString());
        EnvManager.getInstance().set("MODENV_KEY_LOCATION", privKeyPath.toString());
        EnvManager.getInstance().set("MODENV_CLOUD_SECRET_MY_DB_SECRET", "cloud-resolved-pass");
        EnvManager.getInstance().set("MODENV_CLOUD_SECRET_MY_FULL_SECRET", "full-cloud-pass");

        Map<String, Object> cfg = Modenv.load();
        assertEquals("smart-secrets-app", cfg.get("app_name"));

        Map<String, Object> db = (Map<String, Object>) cfg.get("database");
        assertNotNull(db);
        assertEquals("local_db_password", db.get("simple_password"));
        assertEquals("local_db_password", db.get("legacy_password"));
        assertEquals("pks-decrypted-secret-42", db.get("pks_password"));
        assertEquals("cloud-resolved-pass", db.get("cloud_password"));
        assertEquals("full-cloud-pass", db.get("cloud_full_uri"));
    }

    @Test
    public void testPKSWithPKCS1PrivateKey() throws Exception {
        java.security.KeyPairGenerator kpg = java.security.KeyPairGenerator.getInstance("RSA");
        kpg.initialize(2048);
        java.security.KeyPair kp = kpg.generateKeyPair();
        byte[] pkcs8 = kp.getPrivate().getEncoded();

        // Parse PKCS#8 ASN.1 structure: SEQUENCE { version, algId, OCTET STRING { pkcs1 } }
        int offset = 0;
        offset++; // 0x30
        if ((pkcs8[offset] & 0x80) != 0) {
            int n = pkcs8[offset] & 0x7f;
            offset += 1 + n;
        } else {
            offset++;
        }
        offset += 3; // Version (02 01 00)
        offset++; // 0x30 (AlgId)
        int algLen = pkcs8[offset] & 0xff;
        offset += 1 + algLen;
        offset++; // 0x04 (OCTET STRING)
        int octLen;
        if ((pkcs8[offset] & 0x80) != 0) {
            int n = pkcs8[offset] & 0x7f;
            offset++;
            octLen = 0;
            for (int i = 0; i < n; i++) {
                octLen = (octLen << 8) | (pkcs8[offset + i] & 0xff);
            }
            offset += n;
        } else {
            octLen = pkcs8[offset] & 0xff;
            offset++;
        }
        byte[] pkcs1 = new byte[octLen];
        System.arraycopy(pkcs8, offset, pkcs1, 0, octLen);
        String pkcs1Pem = "-----BEGIN RSA PRIVATE KEY-----\n" + Base64.getMimeEncoder(64, new byte[]{'\n'}).encodeToString(pkcs1) + "\n-----END RSA PRIVATE KEY-----";
        String pubPem = "-----BEGIN PUBLIC KEY-----\n" + Base64.getMimeEncoder(64, new byte[]{'\n'}).encodeToString(kp.getPublic().getEncoded()) + "\n-----END PUBLIC KEY-----";

        String enc = Modenv.encryptPKSSecret("pkcs1-hello", pubPem);
        EnvManager.getInstance().set("MODENV_PRIVATE_KEY", pkcs1Pem);
        String dec = Modenv.decryptPKSSecret(enc);
        assertEquals("pkcs1-hello", dec);
    }

    @Test
    public void testPKSWithRelativeKeyPath() throws Exception {
        Path tmpDir = Files.createTempDirectory("modenv-pks-rel");
        Path keysDir = tmpDir.resolve("keys");
        Files.createDirectories(keysDir);
        Path configsDir = getTestConfigsDir();
        String privPem = Files.readString(configsDir.resolve("test_rsa_private.pem"));
        String pubPem = Files.readString(configsDir.resolve("test_rsa_public.pem"));
        Files.writeString(keysDir.resolve("key.pem"), privPem);

        EnvManager.getInstance().set("MODENV_PREFIX", tmpDir.toString());
        EnvManager.getInstance().unset("MODENV_PRIVATE_KEY");
        EnvManager.getInstance().unset("MODENV_KEY_LOCATION");

        String enc = Modenv.encryptPKSSecret("rel-pks", pubPem);
        Modenv.SecretsStoreConfig cfg = new Modenv.SecretsStoreConfig("pks", "proj", "reg", "keys/key.pem");
        String dec = Modenv.decryptPKSSecret(enc, cfg);
        assertEquals("rel-pks", dec);
    }

    @Test
    public void testSecretsStoreConfigAndHelpers() {
        Modenv.SecretsStoreConfig cfg = new Modenv.SecretsStoreConfig();
        cfg.setType("pks");
        cfg.setGoogleCloudProject("p");
        cfg.setGoogleCloudRegion("r");
        cfg.setKeyLocation("k");
        assertEquals("pks", cfg.getType());
        assertEquals("p", cfg.getGoogleCloudProject());
        assertEquals("r", cfg.getGoogleCloudRegion());
        assertEquals("k", cfg.getKeyLocation());

        EnvManager.getInstance().unset("MODENV_PREFIX");
        assertEquals(Paths.get("file.txt"), Modenv.resolvePath("file.txt"));
    }
}
