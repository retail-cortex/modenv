// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package modenv

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

// EnvManager tracks changes to the environment variables and allows reverting them.
type EnvManager struct {
	mu       sync.Mutex
	original map[string]*string // maps key to original value. nil value means it did not exist.
}

// New creates a new EnvManager ready to track environment changes.
func New() *EnvManager {
	return &EnvManager{
		original: make(map[string]*string),
	}
}

// Set sets an environment variable and records its original value if not already tracked.
func (m *EnvManager) Set(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, tracked := m.original[key]; !tracked {
		if val, exists := os.LookupEnv(key); exists {
			m.original[key] = &val
		} else {
			m.original[key] = nil
		}
	}

	return os.Setenv(key, value)
}

// Get retrieves the value of the environment variable named by the key.
func (m *EnvManager) Get(key string) string {
	return os.Getenv(key)
}

// Lookup retrieves the value of the environment variable named by the key and reports if it was present.
func (m *EnvManager) Lookup(key string) (string, bool) {
	return os.LookupEnv(key)
}

// Unset unsets an environment variable and records its original value if not already tracked.
func (m *EnvManager) Unset(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, tracked := m.original[key]; !tracked {
		if val, exists := os.LookupEnv(key); exists {
			m.original[key] = &val
		} else {
			m.original[key] = nil
		}
	}

	return os.Unsetenv(key)
}

// Restore reverts all changes made via this EnvManager to their original values.
func (m *EnvManager) Restore() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for key, valPtr := range m.original {
		var err error
		if valPtr == nil {
			err = os.Unsetenv(key)
		} else {
			err = os.Setenv(key, *valPtr)
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Reset tracking map
	m.original = make(map[string]*string)
	return firstErr
}

// Load reads and parses hierarchical TOML environment configurations:
// 1. .env.toml (required)
// 2. .env.${MODENV_RUNTIME}.toml (optional runtime override)
// 3. .env.local.toml (optional local override, loaded last)
//
// Paths are resolved relative to MODENV_PREFIX if it is set.
// If target is nil, it returns a map[string]interface{}.
// If target is a non-nil pointer to a struct or map, it decodes the configuration into target
// and returns a clean, defensive copy of the decoded object.
func Load(target interface{}) (interface{}, error) {
	// 1. Load .env.toml (required)
	baseFile := resolvePath(".env.toml")
	merged, err := loadFile(baseFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load base config %s: %w", baseFile, err)
	}

	// 2. Load .env.${MODENV_RUNTIME}.toml (optional runtime overrides)
	if runtime := os.Getenv("MODENV_RUNTIME"); runtime != "" {
		runtimeFile := fmt.Sprintf(".env.%s.toml", runtime)
		runtimePath := resolvePath(runtimeFile)
		if fileExists(runtimePath) {
			runtimeMap, err := loadFile(runtimePath)
			if err != nil {
				return nil, fmt.Errorf("failed to load runtime config %s: %w", runtimePath, err)
			}
			merged = deepMerge(merged, runtimeMap)
		}
	}

	// 3. Load .env.local.toml (optional local overrides, loaded last)
	localPath := resolvePath(".env.local.toml")
	if fileExists(localPath) {
		localMap, err := loadFile(localPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load local config %s: %w", localPath, err)
		}
		merged = deepMerge(merged, localMap)
	}

	// Extract secrets_store configuration if present
	storeCfg := extractSecretsStoreConfig(merged)

	// Decrypt configuration secrets in place using smart URI prefix resolution
	if err := decryptConfigMap(merged, storeCfg); err != nil {
		return nil, fmt.Errorf("failed to decrypt secrets in merged configuration: %w", err)
	}

	// 4. Encode the merged map to TOML format
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(merged); err != nil {
		return nil, fmt.Errorf("failed to encode merged configuration: %w", err)
	}
	tomlStr := buf.String()

	// 5. Decode into target or map and return defensive copy
	if target != nil {
		// Populate user target
		_, err := toml.Decode(tomlStr, target)
		if err != nil {
			return nil, fmt.Errorf("failed to decode merged config into target: %w", err)
		}
		// Return defensive copy
		return clone(target)
	}

	// target is nil, decode into a fresh map and return it
	var res map[string]interface{}
	_, err = toml.Decode(tomlStr, &res)
	if err != nil {
		return nil, fmt.Errorf("failed to decode merged config into result map: %w", err)
	}
	return res, nil
}

// SecretsStoreConfig contains settings for secret resolution.
type SecretsStoreConfig struct {
	Type               string `toml:"type"`
	GoogleCloudProject string `toml:"google_cloud_project"`
	GoogleCloudRegion  string `toml:"google_cloud_region"`
	KeyLocation        string `toml:"key_location"`
}

// CloudSecretResolverFunc is a hook for custom cloud secret resolution (e.g. testing).
type CloudSecretResolverFunc func(project, secret, version string) (string, error)

var (
	cloudResolverMu     sync.RWMutex
	customCloudResolver CloudSecretResolverFunc
)

// SetCloudSecretResolver sets a custom resolver for cloud:// secrets.
func SetCloudSecretResolver(resolver CloudSecretResolverFunc) {
	cloudResolverMu.Lock()
	defer cloudResolverMu.Unlock()
	customCloudResolver = resolver
}

// EncryptSecret encrypts a plain text string using XOR and returns a string prefixed with "simple://".
func EncryptSecret(plainText string) string {
	key := getSecretKey()
	input := []byte(plainText)
	output := make([]byte, len(input))
	for i := 0; i < len(input); i++ {
		output[i] = input[i] ^ key[i%len(key)]
	}
	return "simple://" + hex.EncodeToString(output)
}

// EncryptLegacySecret encrypts a plain text string using XOR and returns a string prefixed with "xor:".
func EncryptLegacySecret(plainText string) string {
	key := getSecretKey()
	input := []byte(plainText)
	output := make([]byte, len(input))
	for i := 0; i < len(input); i++ {
		output[i] = input[i] ^ key[i%len(key)]
	}
	return "xor:" + hex.EncodeToString(output)
}

// DecryptSecret decrypts a string starting with "simple://" or "xor:" and returns the plain text.
func DecryptSecret(encodedText string) (string, error) {
	var hexStr string
	if strings.HasPrefix(encodedText, "simple://") {
		hexStr = encodedText[9:]
	} else if strings.HasPrefix(encodedText, "xor:") {
		hexStr = encodedText[4:]
	} else {
		return "", fmt.Errorf("invalid secret format: missing 'simple://' or 'xor:' prefix")
	}
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return "", fmt.Errorf("failed to hex decode secret: %w", err)
	}
	key := getSecretKey()
	output := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		output[i] = data[i] ^ key[i%len(key)]
	}
	return string(output), nil
}

// EncryptPKSSecret encrypts a plaintext using an RSA public key PEM.
func EncryptPKSSecret(plainText string, pubKeyPEM string) (string, error) {
	block, _ := pem.Decode([]byte(pubKeyPEM))
	if block == nil {
		return "", fmt.Errorf("failed to parse PEM block containing public key")
	}
	pubKeyInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		var err2 error
		pubKeyInterface, err2 = x509.ParsePKCS1PublicKey(block.Bytes)
		if err2 != nil {
			return "", fmt.Errorf("failed to parse RSA public key: %w", err)
		}
	}
	rsaPub, ok := pubKeyInterface.(*rsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("parsed key is not an RSA public key")
	}
	cipherBytes, err := rsa.EncryptPKCS1v15(rand.Reader, rsaPub, []byte(plainText))
	if err != nil {
		return "", fmt.Errorf("RSA encryption failed: %w", err)
	}
	return "pks://" + base64.StdEncoding.EncodeToString(cipherBytes), nil
}

// DecryptPKSSecret decrypts a base64 encoded string prefixed with "pks://" using an RSA private key.
func DecryptPKSSecret(encodedText string, cfg SecretsStoreConfig) (string, error) {
	if !strings.HasPrefix(encodedText, "pks://") {
		return "", fmt.Errorf("invalid PKS secret format: missing 'pks://' prefix")
	}
	b64Str := encodedText[6:]
	cipherBytes, err := base64.StdEncoding.DecodeString(b64Str)
	if err != nil {
		return "", fmt.Errorf("failed to base64 decode PKS secret: %w", err)
	}

	privKey, err := loadRSAPrivateKey(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to load private key for PKS secret: %w", err)
	}

	decrypted, err := rsa.DecryptPKCS1v15(nil, privKey, cipherBytes)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt PKS secret: %w", err)
	}
	return string(decrypted), nil
}

// loadRSAPrivateKey loads and parses an RSA private key from env or file path.
func loadRSAPrivateKey(cfg SecretsStoreConfig) (*rsa.PrivateKey, error) {
	var pemData []byte
	if pemEnv := os.Getenv("MODENV_PRIVATE_KEY"); pemEnv != "" {
		pemData = []byte(pemEnv)
	} else {
		keyLoc := cfg.KeyLocation
		if keyLoc == "" {
			keyLoc = os.Getenv("MODENV_KEY_LOCATION")
		}
		if keyLoc == "" {
			return nil, fmt.Errorf("no key_location configured in [secrets_store] and neither MODENV_PRIVATE_KEY nor MODENV_KEY_LOCATION is set")
		}
		targetPath := keyLoc
		if !filepath.IsAbs(targetPath) {
			prefix := os.Getenv("MODENV_PREFIX")
			candidates := []string{
				targetPath,
			}
			if prefix != "" {
				candidates = append([]string{
					filepath.Join(prefix, targetPath),
					filepath.Join(filepath.Dir(prefix), targetPath),
					filepath.Join(filepath.Dir(prefix), filepath.Base(targetPath)),
				}, candidates...)
			}
			found := false
			for _, c := range candidates {
				if fileExists(c) {
					targetPath = c
					found = true
					break
				}
			}
			if !found && prefix != "" {
				targetPath = filepath.Join(prefix, targetPath)
			}
		}
		data, err := os.ReadFile(targetPath)
		if err != nil {
			return nil, fmt.Errorf("could not read private key file at %s: %w", targetPath, err)
		}
		pemData = data
	}

	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block for private key")
	}
	if keyInterface, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := keyInterface.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
	}
	if rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return rsaKey, nil
	}
	return nil, fmt.Errorf("failed to parse RSA private key from PEM")
}

// ResolveCloudSecret resolves a secret from Google Cloud Secret Manager.
func ResolveCloudSecret(uri string, cfg SecretsStoreConfig) (string, error) {
	if !strings.HasPrefix(uri, "cloud://") {
		return "", fmt.Errorf("invalid cloud secret format: missing 'cloud://' prefix")
	}
	raw := uri[8:]
	var project, secret, version string
	if strings.HasPrefix(raw, "projects/") {
		parts := strings.Split(raw, "/")
		if len(parts) >= 4 {
			project = parts[1]
			secret = parts[3]
			if len(parts) >= 6 {
				version = parts[5]
			}
		}
	} else {
		if idx := strings.Index(raw, ":"); idx != -1 {
			secret = raw[:idx]
			version = raw[idx+1:]
		} else {
			secret = raw
		}
		project = cfg.GoogleCloudProject
		if project == "" {
			project = os.Getenv("GOOGLE_CLOUD_PROJECT")
		}
		if project == "" {
			project = os.Getenv("GCP_PROJECT")
		}
		if project == "" {
			project = os.Getenv("MODENV_GCP_PROJECT")
		}
	}
	if version == "" {
		version = "latest"
	}
	if secret == "" {
		return "", fmt.Errorf("cloud secret URI missing secret name: %s", uri)
	}
	if strings.ContainsAny(secret, "/ \r\n") || strings.ContainsAny(project, "/ \r\n") || strings.ContainsAny(version, "/ \r\n") {
		return "", fmt.Errorf("invalid secret, project, or version identifier in cloud secret URI: %s", uri)
	}

	// 1. Check environment variable override for testing/offline: MODENV_CLOUD_SECRET_<SECRET_NAME>
	envKey := "MODENV_CLOUD_SECRET_" + strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(secret, "-", "_"), ".", "_"))
	if envVal, ok := os.LookupEnv(envKey); ok {
		return envVal, nil
	}

	// 2. Check custom resolver hook
	cloudResolverMu.RLock()
	resolver := customCloudResolver
	cloudResolverMu.RUnlock()
	if resolver != nil {
		return resolver(project, secret, version)
	}

	if project == "" {
		return "", fmt.Errorf("cloud secret %q requires google_cloud_project in [secrets_store] or GOOGLE_CLOUD_PROJECT env var", secret)
	}

	// 3. Attempt REST API call
	token := os.Getenv("GOOGLE_OAUTH_ACCESS_TOKEN")
	if token == "" {
		token = os.Getenv("GCP_ACCESS_TOKEN")
	}
	if token == "" {
		client := &http.Client{Timeout: 1500 * time.Millisecond}
		req, _ := http.NewRequest("GET", "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token", nil)
		req.Header.Set("Metadata-Flavor", "Google")
		if resp, err := client.Do(req); err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var metaToken struct {
				AccessToken string `json:"access_token"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&metaToken); err == nil {
				token = metaToken.AccessToken
			}
		}
	}

	if token == "" {
		return "", fmt.Errorf("failed to resolve cloud secret %q: no Google Cloud credentials or %s found", secret, envKey)
	}
	if strings.ContainsAny(token, "\r\n") {
		return "", errors.New("invalid OAuth token: contains newline characters")
	}

	baseURL := os.Getenv("MODENV_GCP_ENDPOINT")
	if baseURL == "" {
		baseURL = "https://secretmanager.googleapis.com"
	}
	apiURL := fmt.Sprintf("%s/v1/projects/%s/secrets/%s/versions/%s:access", baseURL, project, secret, version)
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for cloud secret: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch cloud secret %q: %w", secret, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch cloud secret %q: HTTP %d", secret, resp.StatusCode)
	}
	var res struct {
		Payload struct {
			Data string `json:"data"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("failed to decode cloud secret response: %w", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(res.Payload.Data)
	if err != nil {
		return "", fmt.Errorf("failed to decode payload data for secret %q: %w", secret, err)
	}
	return string(decoded), nil
}

// getSecretKey resolves the encryption key from environment variables with fallback.
func getSecretKey() []byte {
	key := os.Getenv("MODENV_KEY")
	if key == "" {
		key = "modenv-default-key"
	}
	return []byte(key)
}

// resolveSecretValue inspects a string and resolves secret prefixes if matched.
func resolveSecretValue(val string, cfg SecretsStoreConfig) (string, error) {
	if strings.HasPrefix(val, "simple://") || strings.HasPrefix(val, "xor:") {
		return DecryptSecret(val)
	}
	if strings.HasPrefix(val, "pks://") {
		return DecryptPKSSecret(val, cfg)
	}
	if strings.HasPrefix(val, "cloud://") {
		return ResolveCloudSecret(val, cfg)
	}
	return val, nil
}

// extractSecretsStoreConfig parses the [secrets_store] table from the configuration map.
func extractSecretsStoreConfig(m map[string]interface{}) SecretsStoreConfig {
	var cfg SecretsStoreConfig
	if raw, ok := m["secrets_store"].(map[string]interface{}); ok {
		if t, ok := raw["type"].(string); ok {
			cfg.Type = t
		}
		if p, ok := raw["google_cloud_project"].(string); ok {
			cfg.GoogleCloudProject = p
		}
		if r, ok := raw["google_cloud_region"].(string); ok {
			cfg.GoogleCloudRegion = r
		}
		if k, ok := raw["key_location"].(string); ok {
			cfg.KeyLocation = k
		}
	}
	return cfg
}

// decryptConfigMap recursively searches a map for secret values and decrypts them in-place.
func decryptConfigMap(m map[string]interface{}, cfg SecretsStoreConfig) error {
	for k, v := range m {
		switch val := v.(type) {
		case string:
			resolved, err := resolveSecretValue(val, cfg)
			if err != nil {
				return err
			}
			m[k] = resolved
		case map[string]interface{}:
			err := decryptConfigMap(val, cfg)
			if err != nil {
				return err
			}
		case []interface{}:
			err := decryptConfigSlice(val, cfg)
			if err != nil {
				return err
			}
		case []map[string]interface{}:
			for _, item := range val {
				if err := decryptConfigMap(item, cfg); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// decryptConfigSlice recursively searches a slice for secret values and decrypts them in-place.
func decryptConfigSlice(s []interface{}, cfg SecretsStoreConfig) error {
	for i, v := range s {
		switch val := v.(type) {
		case string:
			resolved, err := resolveSecretValue(val, cfg)
			if err != nil {
				return err
			}
			s[i] = resolved
		case map[string]interface{}:
			err := decryptConfigMap(val, cfg)
			if err != nil {
				return err
			}
		case []interface{}:
			err := decryptConfigSlice(val, cfg)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// resolvePath returns the target path joined with MODENV_PREFIX if configured.
func resolvePath(filename string) string {
	prefix := os.Getenv("MODENV_PREFIX")
	if prefix == "" {
		return filename
	}
	return filepath.Join(prefix, filename)
}

// loadFile reads a file and decodes it into a map.
func loadFile(filename string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	m := make(map[string]interface{})
	_, err = toml.Decode(string(data), &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// fileExists checks if a file exists and is not a directory.
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// deepMerge recursively merges src map into dst map.
func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	for k, v := range src {
		if dstVal, exists := dst[k]; exists {
			dstMap, dstIsMap := dstVal.(map[string]interface{})
			srcMap, srcIsMap := v.(map[string]interface{})
			if dstIsMap && srcIsMap {
				dst[k] = deepMerge(dstMap, srcMap)
				continue
			}
		}
		dst[k] = v
	}
	return dst
}

// clone creates a clean, defensive copy of the source pointer object.
func clone(src interface{}) (interface{}, error) {
	if src == nil {
		return nil, nil
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(src); err != nil {
		return nil, err
	}

	val := reflect.ValueOf(src)
	dstPtr := reflect.New(val.Elem().Type())
	if _, err := toml.Decode(buf.String(), dstPtr.Interface()); err != nil {
		return nil, err
	}
	return dstPtr.Interface(), nil
}
