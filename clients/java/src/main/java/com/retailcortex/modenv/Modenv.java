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

import org.tomlj.Toml;
import org.tomlj.TomlArray;
import org.tomlj.TomlParseResult;
import org.tomlj.TomlTable;

import javax.crypto.Cipher;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.io.InputStream;
import java.lang.reflect.Field;
import java.lang.reflect.Modifier;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.security.KeyFactory;
import java.security.PrivateKey;
import java.security.PublicKey;
import java.security.spec.PKCS8EncodedKeySpec;
import java.security.spec.X509EncodedKeySpec;
import java.util.ArrayList;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * Hierarchical TOML configuration and environment loader for Java.
 */
public class Modenv {

    /**
     * Configuration for secrets resolution store.
     */
    public static class SecretsStoreConfig {
        private String type = "simple";
        private String googleCloudProject = "";
        private String googleCloudRegion = "";
        private String keyLocation = "";

        public SecretsStoreConfig() {}

        public SecretsStoreConfig(String type, String googleCloudProject, String googleCloudRegion, String keyLocation) {
            this.type = type != null ? type : "simple";
            this.googleCloudProject = googleCloudProject != null ? googleCloudProject : "";
            this.googleCloudRegion = googleCloudRegion != null ? googleCloudRegion : "";
            this.keyLocation = keyLocation != null ? keyLocation : "";
        }

        public String getType() { return type; }
        public void setType(String type) { this.type = type; }
        public String getGoogleCloudProject() { return googleCloudProject; }
        public void setGoogleCloudProject(String googleCloudProject) { this.googleCloudProject = googleCloudProject; }
        public String getGoogleCloudRegion() { return googleCloudRegion; }
        public void setGoogleCloudRegion(String googleCloudRegion) { this.googleCloudRegion = googleCloudRegion; }
        public String getKeyLocation() { return keyLocation; }
        public void setKeyLocation(String keyLocation) { this.keyLocation = keyLocation; }
    }

    @FunctionalInterface
    public interface CloudSecretResolver {
        String resolve(String uri, SecretsStoreConfig config) throws Exception;
    }

    private static CloudSecretResolver cloudSecretResolver = null;

    public static void setCloudSecretResolver(CloudSecretResolver resolver) {
        cloudSecretResolver = resolver;
    }

    private Modenv() {}

    /**
     * Encrypts a plain text string using XOR cipher and returns hex encoded string prefixed with 'simple://'.
     */
    public static String encryptSecret(String plainText) {
        byte[] key = getSecretKey();
        byte[] input = plainText.getBytes(StandardCharsets.UTF_8);
        byte[] output = new byte[input.length];
        for (int i = 0; i < input.length; i++) {
            output[i] = (byte) (input[i] ^ key[i % key.length]);
        }
        return "simple://" + bytesToHex(output);
    }

    /**
     * Encrypts a plain text string using XOR cipher and returns hex encoded string prefixed with legacy 'xor:'.
     */
    public static String encryptLegacySecret(String plainText) {
        byte[] key = getSecretKey();
        byte[] input = plainText.getBytes(StandardCharsets.UTF_8);
        byte[] output = new byte[input.length];
        for (int i = 0; i < input.length; i++) {
            output[i] = (byte) (input[i] ^ key[i % key.length]);
        }
        return "xor:" + bytesToHex(output);
    }

    /**
     * Decrypts an encoded string starting with 'simple://' or 'xor:' and returns the plain text.
     */
    public static String decryptSecret(String encodedText) {
        if (encodedText == null) {
            throw new IllegalArgumentException("secret cannot be null");
        }
        String hexStr;
        if (encodedText.startsWith("simple://")) {
            hexStr = encodedText.substring(9);
        } else if (encodedText.startsWith("xor:")) {
            hexStr = encodedText.substring(4);
        } else {
            throw new IllegalArgumentException("invalid secret format: missing 'simple://' or 'xor:' prefix");
        }
        byte[] data = hexToBytes(hexStr);
        byte[] key = getSecretKey();
        byte[] output = new byte[data.length];
        for (int i = 0; i < data.length; i++) {
            output[i] = (byte) (data[i] ^ key[i % key.length]);
        }
        return new String(output, StandardCharsets.UTF_8);
    }

    /**
     * Encrypts a plain text string using an RSA public key PEM and PKCS#1 v1.5 padding.
     */
    public static String encryptPKSSecret(String plainText, String pubKeyPEM) {
        try {
            PublicKey pubKey = parsePublicKey(pubKeyPEM);
            Cipher cipher = Cipher.getInstance("RSA/ECB/PKCS1Padding");
            cipher.init(Cipher.ENCRYPT_MODE, pubKey);
            byte[] cipherBytes = cipher.doFinal(plainText.getBytes(StandardCharsets.UTF_8));
            return "pks://" + Base64.getEncoder().encodeToString(cipherBytes);
        } catch (Exception e) {
            throw new RuntimeException("RSA encryption failed: " + e.getMessage(), e);
        }
    }

    /**
     * Decrypts a 'pks://' prefixed base64 RSA encrypted secret.
     */
    public static String decryptPKSSecret(String encodedText) {
        return decryptPKSSecret(encodedText, null);
    }

    /**
     * Decrypts a 'pks://' prefixed base64 RSA encrypted secret using store config or environment fallback.
     */
    public static String decryptPKSSecret(String encodedText, SecretsStoreConfig cfg) {
        if (encodedText == null || !encodedText.startsWith("pks://")) {
            throw new IllegalArgumentException("invalid secret format: missing 'pks://' prefix");
        }
        String rawB64 = encodedText.substring(6);

        String pemData = EnvManager.getInstance().get("MODENV_PRIVATE_KEY");
        if (pemData == null || pemData.isEmpty()) {
            String keyLocation = EnvManager.getInstance().get("MODENV_KEY_LOCATION");
            if ((keyLocation == null || keyLocation.isEmpty()) && cfg != null) {
                keyLocation = cfg.getKeyLocation();
            }
            if (keyLocation == null || keyLocation.isEmpty()) {
                throw new IllegalStateException("cannot decrypt PKS secret: no private key found in key_location, MODENV_KEY_LOCATION, or MODENV_PRIVATE_KEY");
            }
            Path targetPath = Paths.get(keyLocation);
            if (!targetPath.isAbsolute() && !Files.exists(targetPath)) {
                String prefix = EnvManager.getInstance().get("MODENV_PREFIX");
                if (prefix != null && !prefix.isEmpty()) {
                    targetPath = Paths.get(prefix, keyLocation);
                }
            }
            if (!Files.exists(targetPath) || Files.isDirectory(targetPath)) {
                throw new RuntimeException("could not read private key file at " + targetPath);
            }
            try {
                pemData = Files.readString(targetPath);
            } catch (IOException e) {
                throw new RuntimeException("failed to read private key file: " + e.getMessage(), e);
            }
        }

        try {
            PrivateKey privKey = parsePrivateKey(pemData);
            Cipher cipher = Cipher.getInstance("RSA/ECB/PKCS1Padding");
            cipher.init(Cipher.DECRYPT_MODE, privKey);
            byte[] cipherBytes = Base64.getDecoder().decode(rawB64);
            byte[] plainBytes = cipher.doFinal(cipherBytes);
            return new String(plainBytes, StandardCharsets.UTF_8);
        } catch (Exception e) {
            throw new RuntimeException("RSA decryption failed: " + e.getMessage(), e);
        }
    }

    private static PublicKey parsePublicKey(String pem) throws Exception {
        String clean = pem.replaceAll("-----[A-Z ]+-----", "").replaceAll("\\s+", "");
        byte[] der = Base64.getDecoder().decode(clean);
        KeyFactory kf = KeyFactory.getInstance("RSA");
        return kf.generatePublic(new X509EncodedKeySpec(der));
    }

    private static PrivateKey parsePrivateKey(String pem) throws Exception {
        String clean = pem.replaceAll("-----[A-Z ]+-----", "").replaceAll("\\s+", "");
        byte[] der = Base64.getDecoder().decode(clean);
        KeyFactory kf = KeyFactory.getInstance("RSA");
        try {
            return kf.generatePrivate(new PKCS8EncodedKeySpec(der));
        } catch (Exception e) {
            byte[] pkcs8 = wrapPKCS1ToPKCS8(der);
            return kf.generatePrivate(new PKCS8EncodedKeySpec(pkcs8));
        }
    }

    private static byte[] wrapPKCS1ToPKCS8(byte[] pkcs1) {
        byte[] algId = new byte[] {
            0x30, 0x0d, 0x06, 0x09, 0x2a, (byte) 0x86, 0x48, (byte) 0x86,
            (byte) 0xf7, 0x0d, 0x01, 0x01, 0x01, 0x05, 0x00
        };
        byte[] octetString = encodeDer(0x04, pkcs1);
        byte[] version = new byte[] { 0x02, 0x01, 0x00 };
        byte[] body = new byte[version.length + algId.length + octetString.length];
        System.arraycopy(version, 0, body, 0, version.length);
        System.arraycopy(algId, 0, body, version.length, algId.length);
        System.arraycopy(octetString, 0, body, version.length + algId.length, octetString.length);
        return encodeDer(0x30, body);
    }

    private static byte[] encodeDer(int tag, byte[] content) {
        byte[] lenBytes;
        if (content.length < 128) {
            lenBytes = new byte[] { (byte) content.length };
        } else if (content.length < 256) {
            lenBytes = new byte[] { (byte) 0x81, (byte) content.length };
        } else {
            lenBytes = new byte[] {
                (byte) 0x82,
                (byte) ((content.length >> 8) & 0xff),
                (byte) (content.length & 0xff)
            };
        }
        byte[] out = new byte[1 + lenBytes.length + content.length];
        out[0] = (byte) tag;
        System.arraycopy(lenBytes, 0, out, 1, lenBytes.length);
        System.arraycopy(content, 0, out, 1 + lenBytes.length, content.length);
        return out;
    }

    /**
     * Resolves a secret from Google Cloud Secret Manager URI with default configuration.
     */
    public static String resolveCloudSecret(String uri) {
        return resolveCloudSecret(uri, null);
    }

    /**
     * Resolves a secret from Google Cloud Secret Manager URI.
     */
    public static String resolveCloudSecret(String uri, SecretsStoreConfig cfg) {
        if (uri == null || !uri.startsWith("cloud://")) {
            throw new IllegalArgumentException("invalid cloud secret URI: " + uri);
        }
        String trimmed = uri.substring(8);
        String project = "";
        String secret = "";
        String version = "latest";

        Pattern p = Pattern.compile("^projects/([^/]+)/secrets/([^/]+)(?:/versions/([^/]+))?$");
        Matcher m = p.matcher(trimmed);
        if (m.matches()) {
            project = m.group(1);
            secret = m.group(2);
            if (m.group(3) != null) {
                version = m.group(3);
            }
        } else {
            if (trimmed.contains(":")) {
                String[] parts = trimmed.split(":", 2);
                secret = parts[0];
                version = parts[1];
            } else {
                secret = trimmed;
            }
            if (cfg != null && cfg.getGoogleCloudProject() != null && !cfg.getGoogleCloudProject().isEmpty()) {
                project = cfg.getGoogleCloudProject();
            }
            if (project.isEmpty()) {
                project = EnvManager.getInstance().get("GOOGLE_CLOUD_PROJECT");
                if (project == null || project.isEmpty()) {
                    project = EnvManager.getInstance().get("GCP_PROJECT");
                }
                if (project == null || project.isEmpty()) {
                    project = EnvManager.getInstance().get("MODENV_GCP_PROJECT");
                }
            }
        }

        if (project == null || project.isEmpty()) {
            throw new IllegalArgumentException("cannot resolve cloud secret " + uri + ": Google Cloud Project ID is not configured");
        }

        if (secret.isEmpty()) {
            throw new IllegalArgumentException("cloud secret URI missing secret name: " + uri);
        }
        if (secret.contains("/") || secret.contains(" ") || secret.contains("\r") || secret.contains("\n") ||
            project.contains("/") || project.contains(" ") || project.contains("\r") || project.contains("\n") ||
            version.contains("/") || version.contains(" ") || version.contains("\r") || version.contains("\n")) {
            throw new IllegalArgumentException("invalid secret, project, or version identifier in cloud secret URI: " + uri);
        }

        if (cloudSecretResolver != null) {
            try {
                return cloudSecretResolver.resolve(uri, cfg != null ? cfg : new SecretsStoreConfig());
            } catch (Exception e) {
                throw new RuntimeException("custom cloud secret resolver failed: " + e.getMessage(), e);
            }
        }

        String envOverride = EnvManager.getInstance().get("MODENV_CLOUD_SECRET_" + secret.toUpperCase().replace("-", "_"));
        if (envOverride != null && !envOverride.isEmpty()) {
            return envOverride;
        }

        String token = EnvManager.getInstance().get("GOOGLE_OAUTH_ACCESS_TOKEN");
        if (token == null || token.isEmpty()) {
            token = EnvManager.getInstance().get("GCP_ACCESS_TOKEN");
        }
        if (token == null || token.isEmpty()) {
            token = fetchMetadataToken();
        }
        if (token == null || token.isEmpty()) {
            throw new RuntimeException("failed to resolve cloud secret '" + secret + "': no Google Cloud credentials or MODENV_CLOUD_SECRET_" + secret.toUpperCase().replace("-", "_") + " found");
        }
        if (token.contains("\r") || token.contains("\n")) {
            throw new IllegalArgumentException("invalid OAuth token: contains newline characters");
        }

        String baseURL = EnvManager.getInstance().get("MODENV_GCP_ENDPOINT");
        if (baseURL == null || baseURL.isEmpty()) {
            baseURL = "https://secretmanager.googleapis.com";
        }
        String apiURL = baseURL + "/v1/projects/" + project + "/secrets/" + secret + "/versions/" + version + ":access";

        try {
            HttpURLConnection conn = (HttpURLConnection) new URL(apiURL).openConnection();
            conn.setRequestMethod("GET");
            conn.setRequestProperty("Authorization", "Bearer " + token);
            conn.setConnectTimeout(5000);
            conn.setReadTimeout(5000);

            int code = conn.getResponseCode();
            if (code != 200) {
                throw new RuntimeException("failed to fetch cloud secret '" + secret + "': HTTP status " + code);
            }
            try (InputStream is = conn.getInputStream()) {
                String responseBody = new String(is.readAllBytes(), StandardCharsets.UTF_8);
                Pattern dataPattern = Pattern.compile("\"data\"\\s*:\\s*\"([^\"]+)\"");
                Matcher dataMatcher = dataPattern.matcher(responseBody);
                if (!dataMatcher.find()) {
                    throw new RuntimeException("missing payload.data in Secret Manager response");
                }
                String b64Data = dataMatcher.group(1);
                return new String(Base64.getDecoder().decode(b64Data), StandardCharsets.UTF_8);
            }
        } catch (Exception e) {
            throw new RuntimeException("failed to fetch cloud secret '" + secret + "': " + e.getMessage(), e);
        }
    }

    private static String fetchMetadataToken() {
        try {
            HttpURLConnection conn = (HttpURLConnection) new URL("http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token").openConnection();
            conn.setRequestMethod("GET");
            conn.setRequestProperty("Metadata-Flavor", "Google");
            conn.setConnectTimeout(1500);
            conn.setReadTimeout(1500);
            if (conn.getResponseCode() == 200) {
                try (InputStream is = conn.getInputStream()) {
                    String json = new String(is.readAllBytes(), StandardCharsets.UTF_8);
                    Pattern p = Pattern.compile("\"access_token\"\\s*:\\s*\"([^\"]+)\"");
                    Matcher m = p.matcher(json);
                    if (m.find()) {
                        return m.group(1);
                    }
                }
            }
        } catch (Exception ignored) {}
        return null;
    }

    private static byte[] getSecretKey() {
        String key = EnvManager.getInstance().get("MODENV_KEY");
        if (key == null || key.isEmpty()) {
            key = "modenv-default-key";
        }
        return key.getBytes(StandardCharsets.UTF_8);
    }

    private static String bytesToHex(byte[] bytes) {
        StringBuilder sb = new StringBuilder(bytes.length * 2);
        for (byte b : bytes) {
            sb.append(String.format("%02x", b));
        }
        return sb.toString();
    }

    private static byte[] hexToBytes(String hex) {
        if (hex.length() % 2 != 0) {
            throw new IllegalArgumentException("invalid hex string length");
        }
        int len = hex.length();
        byte[] data = new byte[len / 2];
        for (int i = 0; i < len; i += 2) {
            int high = Character.digit(hex.charAt(i), 16);
            int low = Character.digit(hex.charAt(i + 1), 16);
            if (high == -1 || low == -1) {
                throw new IllegalArgumentException("invalid hex character in string");
            }
            data[i / 2] = (byte) ((high << 4) + low);
        }
        return data;
    }

    public static Path resolvePath(String filename) {
        String prefix = EnvManager.getInstance().get("MODENV_PREFIX");
        if (prefix == null || prefix.isEmpty()) {
            return Paths.get(filename);
        }
        return Paths.get(prefix, filename);
    }

    /**
     * Loads and parses hierarchical TOML configurations:
     * 1. .env.toml (required)
     * 2. .env.${MODENV_RUNTIME}.toml (optional runtime override)
     * 3. .env.local.toml (optional local override)
     */
    public static Map<String, Object> load() throws IOException {
        return load(null);
    }

    /**
     * Loads configuration into the provided target object and returns a defensive copy.
     */
    @SuppressWarnings("unchecked")
    public static <T> T load(T target) throws IOException {
        Path basePath = resolvePath(".env.toml");
        if (!Files.exists(basePath) || Files.isDirectory(basePath)) {
            throw new FileNotFoundException("failed to load base config " + basePath + ": file not found");
        }

        Map<String, Object> merged = loadFile(basePath);

        String runtime = EnvManager.getInstance().get("MODENV_RUNTIME");
        if (runtime != null && !runtime.isEmpty()) {
            Path runtimePath = resolvePath(".env." + runtime + ".toml");
            if (Files.exists(runtimePath) && !Files.isDirectory(runtimePath)) {
                Map<String, Object> runtimeMap = loadFile(runtimePath);
                merged = deepMerge(merged, runtimeMap);
            }
        }

        Path localPath = resolvePath(".env.local.toml");
        if (Files.exists(localPath) && !Files.isDirectory(localPath)) {
            Map<String, Object> localMap = loadFile(localPath);
            merged = deepMerge(merged, localMap);
        }

        SecretsStoreConfig cfg = new SecretsStoreConfig();
        if (merged.containsKey("secrets_store") && merged.get("secrets_store") instanceof Map) {
            Map<String, Object> storeMap = (Map<String, Object>) merged.get("secrets_store");
            cfg.setType(String.valueOf(storeMap.getOrDefault("type", "simple")));
            cfg.setGoogleCloudProject(String.valueOf(storeMap.getOrDefault("google_cloud_project", "")));
            cfg.setGoogleCloudRegion(String.valueOf(storeMap.getOrDefault("google_cloud_region", "")));
            cfg.setKeyLocation(String.valueOf(storeMap.getOrDefault("key_location", "")));
        }

        decryptConfigInPlace(merged, cfg);

        if (target == null) {
            return (T) deepCopy(merged);
        }

        if (target instanceof Map) {
            Map rawMap = (Map) target;
            rawMap.clear();
            rawMap.putAll(deepCopy(merged));
            return (T) deepCopy(rawMap);
        }

        bindToObject(target, merged);
        return deepCloneObject(target);
    }

    private static Map<String, Object> loadFile(Path path) throws IOException {
        TomlParseResult result = Toml.parse(path);
        if (result.hasErrors()) {
            throw new IOException("Failed to parse TOML file " + path + ": " + result.errors().get(0));
        }
        return convertTomlTable(result);
    }

    private static Map<String, Object> convertTomlTable(TomlTable table) {
        Map<String, Object> map = new LinkedHashMap<>();
        for (String key : table.keySet()) {
            Object val = table.get(key);
            if (val instanceof TomlTable) {
                map.put(key, convertTomlTable((TomlTable) val));
            } else if (val instanceof TomlArray) {
                map.put(key, convertTomlArray((TomlArray) val));
            } else {
                map.put(key, val);
            }
        }
        return map;
    }

    private static List<Object> convertTomlArray(TomlArray array) {
        List<Object> list = new ArrayList<>();
        for (int i = 0; i < array.size(); i++) {
            Object val = array.get(i);
            if (val instanceof TomlTable) {
                list.add(convertTomlTable((TomlTable) val));
            } else if (val instanceof TomlArray) {
                list.add(convertTomlArray((TomlArray) val));
            } else {
                list.add(val);
            }
        }
        return list;
    }

    @SuppressWarnings("unchecked")
    public static Map<String, Object> deepMerge(Map<String, Object> dst, Map<String, Object> src) {
        Map<String, Object> result = new LinkedHashMap<>(dst);
        for (Map.Entry<String, Object> entry : src.entrySet()) {
            String key = entry.getKey();
            Object srcVal = entry.getValue();
            if (result.containsKey(key)) {
                Object dstVal = result.get(key);
                if (dstVal instanceof Map && srcVal instanceof Map) {
                    result.put(key, deepMerge((Map<String, Object>) dstVal, (Map<String, Object>) srcVal));
                    continue;
                }
            }
            result.put(key, srcVal);
        }
        return result;
    }

    @SuppressWarnings("unchecked")
    private static void decryptConfigInPlace(Object obj, SecretsStoreConfig cfg) {
        if (obj instanceof Map) {
            Map<String, Object> map = (Map<String, Object>) obj;
            for (Map.Entry<String, Object> entry : map.entrySet()) {
                Object val = entry.getValue();
                if (val instanceof String) {
                    String s = (String) val;
                    if (s.startsWith("simple://") || s.startsWith("xor:")) {
                        entry.setValue(decryptSecret(s));
                    } else if (s.startsWith("pks://")) {
                        entry.setValue(decryptPKSSecret(s, cfg));
                    } else if (s.startsWith("cloud://")) {
                        entry.setValue(resolveCloudSecret(s, cfg));
                    }
                } else if (val instanceof Map || val instanceof List) {
                    decryptConfigInPlace(val, cfg);
                }
            }
        } else if (obj instanceof List) {
            List<Object> list = (List<Object>) obj;
            for (int i = 0; i < list.size(); i++) {
                Object val = list.get(i);
                if (val instanceof String) {
                    String s = (String) val;
                    if (s.startsWith("simple://") || s.startsWith("xor:")) {
                        list.set(i, decryptSecret(s));
                    } else if (s.startsWith("pks://")) {
                        list.set(i, decryptPKSSecret(s, cfg));
                    } else if (s.startsWith("cloud://")) {
                        list.set(i, resolveCloudSecret(s, cfg));
                    }
                } else if (val instanceof Map || val instanceof List) {
                    decryptConfigInPlace(val, cfg);
                }
            }
        }
    }

    @SuppressWarnings("unchecked")
    public static Map<String, Object> deepCopy(Map<String, Object> src) {
        Map<String, Object> copy = new LinkedHashMap<>();
        for (Map.Entry<String, Object> entry : src.entrySet()) {
            Object val = entry.getValue();
            if (val instanceof Map) {
                copy.put(entry.getKey(), deepCopy((Map<String, Object>) val));
            } else if (val instanceof List) {
                copy.put(entry.getKey(), deepCopyList((List<Object>) val));
            } else {
                copy.put(entry.getKey(), val);
            }
        }
        return copy;
    }

    @SuppressWarnings("unchecked")
    public static List<Object> deepCopyList(List<Object> src) {
        List<Object> copy = new ArrayList<>();
        for (Object item : src) {
            if (item instanceof Map) {
                copy.add(deepCopy((Map<String, Object>) item));
            } else if (item instanceof List) {
                copy.add(deepCopyList((List<Object>) item));
            } else {
                copy.add(item);
            }
        }
        return copy;
    }

    @SuppressWarnings("unchecked")
    private static void bindToObject(Object target, Map<String, Object> data) {
        if (target == null || data == null) return;
        Class<?> clazz = target.getClass();
        for (Field field : clazz.getDeclaredFields()) {
            if (Modifier.isStatic(field.getModifiers())) continue;
            field.setAccessible(true);
            String fieldName = field.getName();
            String snakeName = toSnakeCase(fieldName);
            Object value = null;
            if (data.containsKey(fieldName)) {
                value = data.get(fieldName);
            } else if (data.containsKey(snakeName)) {
                value = data.get(snakeName);
            }

            if (value == null) continue;

            try {
                Class<?> type = field.getType();
                if (type == int.class || type == Integer.class) {
                    field.set(target, ((Number) value).intValue());
                } else if (type == long.class || type == Long.class) {
                    field.set(target, ((Number) value).longValue());
                } else if (type == double.class || type == Double.class) {
                    field.set(target, ((Number) value).doubleValue());
                } else if (type == boolean.class || type == Boolean.class) {
                    field.set(target, (Boolean) value);
                } else if (type == String.class) {
                    field.set(target, value.toString());
                } else if (type == List.class && value instanceof List) {
                    field.set(target, new ArrayList<>((List<?>) value));
                } else if (value instanceof Map) {
                    Object nested = field.get(target);
                    if (nested == null) {
                        try {
                            nested = type.getDeclaredConstructor().newInstance();
                            field.set(target, nested);
                        } catch (Exception ignored) {}
                    }
                    if (nested != null) {
                        bindToObject(nested, (Map<String, Object>) value);
                    }
                }
            } catch (Exception ignored) {}
        }
    }

    @SuppressWarnings("unchecked")
    private static <T> T deepCloneObject(T obj) {
        if (obj == null) return null;
        try {
            Class<?> clazz = obj.getClass();
            T clone = (T) clazz.getDeclaredConstructor().newInstance();
            for (Field field : clazz.getDeclaredFields()) {
                if (Modifier.isStatic(field.getModifiers())) continue;
                field.setAccessible(true);
                Object val = field.get(obj);
                if (val instanceof List) {
                    field.set(clone, new ArrayList<>((List<?>) val));
                } else if (val != null && !isPrimitiveOrWrapper(val.getClass()) && !(val instanceof String)) {
                    field.set(clone, deepCloneObject(val));
                } else {
                    field.set(clone, val);
                }
            }
            return clone;
        } catch (Exception e) {
            return obj;
        }
    }

    private static boolean isPrimitiveOrWrapper(Class<?> type) {
        return type.isPrimitive() ||
                type == Boolean.class ||
                type == Byte.class ||
                type == Character.class ||
                type == Short.class ||
                type == Integer.class ||
                type == Long.class ||
                type == Float.class ||
                type == Double.class;
    }

    private static String toSnakeCase(String camel) {
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < camel.length(); i++) {
            char c = camel.charAt(i);
            if (Character.isUpperCase(c)) {
                if (i > 0) sb.append('_');
                sb.append(Character.toLowerCase(c));
            } else {
                sb.append(c);
            }
        }
        return sb.toString();
    }
}
