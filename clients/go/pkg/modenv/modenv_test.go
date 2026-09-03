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

package modenv_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rrmcguinness/modenv/pkg/modenv"
	"github.com/stretchr/testify/assert"
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
	Password string `toml:"password"`
}

func getTestConfigsDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "../../test/configs" // fallback
	}
	for {
		// Check for test/configs (when run from workspace root or pkg/)
		targetTestConfigs := filepath.Join(dir, "test", "configs")
		if info, err := os.Stat(targetTestConfigs); err == nil && info.IsDir() {
			return targetTestConfigs
		}
		// Check for configs (when run from test/)
		targetConfigs := filepath.Join(dir, "configs")
		if info, err := os.Stat(targetConfigs); err == nil && info.IsDir() {
			return targetConfigs
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "../../test/configs"
}

func TestSecretEncryptionDecryptionRoundtrip(t *testing.T) {
	plain := "my-super-secret-password-123!"

	// Test default key
	encoded := modenv.EncryptSecret(plain)
	assert.True(t, strings.HasPrefix(encoded, "simple://") || strings.HasPrefix(encoded, "xor:"))
	decoded, err := modenv.DecryptSecret(encoded)
	assert.NoError(t, err)
	assert.Equal(t, plain, decoded)

	// Test custom key
	os.Setenv("MODENV_KEY", "custom-super-long-key-spec")
	defer os.Unsetenv("MODENV_KEY")

	encodedCustom := modenv.EncryptSecret(plain)
	assert.NotEqual(t, encoded, encodedCustom)
	decodedCustom, err := modenv.DecryptSecret(encodedCustom)
	assert.NoError(t, err)
	assert.Equal(t, plain, decodedCustom)
}

func TestLoad_WithEncryptedSecrets(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "secrets")
	t.Setenv("MODENV_PREFIX", dir)

	res, err := modenv.Load(nil)
	assert.NoError(t, err)

	cfgMap, ok := res.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "secret-app", cfgMap["app_name"])

	dbMap, ok := cfgMap["database"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "local_db_password", dbMap["password"])
}

func TestLoad_WithPrefix(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "prefix-test")
	t.Setenv("MODENV_PREFIX", dir)

	res, err := modenv.Load(nil)
	assert.NoError(t, err)

	cfgMap, ok := res.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "prefixed-app", cfgMap["app_name"])
	assert.Equal(t, int64(2000), cfgMap["port"])
}

func TestLoad_MapDefault(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "default")
	t.Setenv("MODENV_PREFIX", dir)

	res, err := modenv.Load(nil)
	assert.NoError(t, err)

	cfgMap, ok := res.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "modenv-default", cfgMap["app_name"])
	assert.Equal(t, int64(8080), cfgMap["port"])

	dbMap, ok := cfgMap["database"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "localhost", dbMap["host"])
}

func TestLoad_Struct(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "struct")
	t.Setenv("MODENV_PREFIX", dir)

	var cfg Config
	cloneRes, err := modenv.Load(&cfg)
	assert.NoError(t, err)

	assert.Equal(t, "modenv-struct", cfg.AppName)
	assert.Equal(t, 9000, cfg.Port)
	assert.Equal(t, []string{"web"}, cfg.Features)
	assert.Equal(t, "db", cfg.Database.Host)

	cloneCfg, ok := cloneRes.(*Config)
	assert.True(t, ok)
	assert.Equal(t, "modenv-struct", cloneCfg.AppName)
	assert.Equal(t, 9000, cloneCfg.Port)
}

func TestLoad_RuntimeOverride(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "runtime")
	t.Setenv("MODENV_PREFIX", dir)
	os.Setenv("MODENV_RUNTIME", "production")
	defer os.Unsetenv("MODENV_RUNTIME")

	res, err := modenv.Load(nil)
	assert.NoError(t, err)

	cfgMap := res.(map[string]interface{})
	assert.Equal(t, "base", cfgMap["app_name"])
	assert.Equal(t, int64(9999), cfgMap["port"])
	dbMap := cfgMap["database"].(map[string]interface{})
	assert.Equal(t, "prod-db", dbMap["host"])
}

func TestLoad_LocalOverrideLast(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "local-override")
	t.Setenv("MODENV_PREFIX", dir)
	os.Setenv("MODENV_RUNTIME", "production")
	defer os.Unsetenv("MODENV_RUNTIME")

	res, err := modenv.Load(nil)
	assert.NoError(t, err)

	cfgMap := res.(map[string]interface{})
	assert.Equal(t, "prod", cfgMap["app_name"])
	assert.Equal(t, int64(7777), cfgMap["port"])
	dbMap := cfgMap["database"].(map[string]interface{})
	assert.Equal(t, "local-db", dbMap["host"])
}

func TestLoad_DefensiveCopy(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "defensive")
	t.Setenv("MODENV_PREFIX", dir)

	var cfg Config
	cloneRes, err := modenv.Load(&cfg)
	assert.NoError(t, err)

	cloneCfg, ok := cloneRes.(*Config)
	assert.True(t, ok)

	assert.Equal(t, "defensive", cfg.AppName)
	assert.Equal(t, "defensive", cloneCfg.AppName)

	cloneCfg.AppName = "mutated"
	cloneCfg.Features[0] = "mutated-feature"
	cloneCfg.Database.Host = "mutated-host"

	assert.Equal(t, "defensive", cfg.AppName)
	assert.Equal(t, "original", cfg.Features[0])
	assert.Equal(t, "original-host", cfg.Database.Host)
}

func TestLoad_MissingBaseFileReturnsError(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "missing")
	t.Setenv("MODENV_PREFIX", dir)

	res, err := modenv.Load(nil)
	assert.Error(t, err)
	assert.Nil(t, res)
}

func TestEnvManager(t *testing.T) {
	const preExistingKey = "TEST_PRE_EXISTING"
	const preExistingVal = "original_value"
	err := os.Setenv(preExistingKey, preExistingVal)
	assert.NoError(t, err)
	defer func() {
		_ = os.Unsetenv(preExistingKey)
	}()

	tests := []struct {
		name     string
		action   func(m *modenv.EnvManager)
		verify   func(t *testing.T, m *modenv.EnvManager)
		expected map[string]string
	}{
		{
			name: "Set new variable",
			action: func(m *modenv.EnvManager) {
				err := m.Set("TEST_NEW_KEY", "new_val")
				assert.NoError(t, err)
			},
			verify: func(t *testing.T, m *modenv.EnvManager) {
				val, exists := m.Lookup("TEST_NEW_KEY")
				assert.True(t, exists)
				assert.Equal(t, "new_val", val)
			},
			expected: map[string]string{
				"TEST_NEW_KEY": "",
			},
		},
		{
			name: "Modify existing variable",
			action: func(m *modenv.EnvManager) {
				err := m.Set(preExistingKey, "modified_val")
				assert.NoError(t, err)
			},
			verify: func(t *testing.T, m *modenv.EnvManager) {
				val, exists := m.Lookup(preExistingKey)
				assert.True(t, exists)
				assert.Equal(t, "modified_val", val)
			},
			expected: map[string]string{
				preExistingKey: preExistingVal,
			},
		},
		{
			name: "Unset existing variable",
			action: func(m *modenv.EnvManager) {
				err := m.Unset(preExistingKey)
				assert.NoError(t, err)
			},
			verify: func(t *testing.T, m *modenv.EnvManager) {
				_, exists := m.Lookup(preExistingKey)
				assert.False(t, exists)
			},
			expected: map[string]string{
				preExistingKey: preExistingVal,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := modenv.New()
			tt.action(m)
			tt.verify(t, m)
			err := m.Restore()
			assert.NoError(t, err)

			for key, expectedVal := range tt.expected {
				val, exists := os.LookupEnv(key)
				if expectedVal == "" {
					assert.False(t, exists, "key %s should not exist after restore", key)
				} else {
					assert.True(t, exists, "key %s should exist after restore", key)
					assert.Equal(t, expectedVal, val)
				}
			}
		})
	}

	// Extra coverage for EnvManager Get and Unset non-existent
	m := modenv.New()
	assert.Equal(t, "", m.Get("NON_EXISTENT_VAR_12345"))
	err = m.Unset("NON_EXISTENT_VAR_12345")
	assert.NoError(t, err)
	err = m.Restore()
	assert.NoError(t, err)
}

func TestDecryptSecretErrors(t *testing.T) {
	_, err := modenv.DecryptSecret("not-prefixed")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'simple://' or 'xor:' prefix")

	_, err = modenv.DecryptSecret("xor:invalid-hex!")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to hex decode secret")
}

func TestDecryptConfigSliceAndNestedStructures(t *testing.T) {
	secret1 := modenv.EncryptSecret("secret-value-1")
	secret2 := modenv.EncryptSecret("secret-value-2")

	// Create a temp directory with TOML containing list of secrets and nested maps in lists
	tmpDir := t.TempDir()
	tomlContent := `
app_name = "slice-test"
port = 8080
secret_list = ["` + secret1 + `", "plain"]
nested_list_of_maps = [
  { key = "` + secret2 + `" }
]
deep_nested = [
  ["` + secret1 + `"]
]
`
	err := os.WriteFile(filepath.Join(tmpDir, ".env.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	t.Setenv("MODENV_PREFIX", tmpDir)
	res, err := modenv.Load(nil)
	assert.NoError(t, err)

	m, ok := res.(map[string]interface{})
	assert.True(t, ok)

	slice1 := m["secret_list"].([]interface{})
	assert.Equal(t, "secret-value-1", slice1[0])
	assert.Equal(t, "plain", slice1[1])

	nestedMaps, okNested := m["nested_list_of_maps"].([]map[string]interface{})
	if okNested {
		assert.Equal(t, "secret-value-2", nestedMaps[0]["key"])
	} else {
		nestedSlice := m["nested_list_of_maps"].([]interface{})
		firstMap := nestedSlice[0].(map[string]interface{})
		assert.Equal(t, "secret-value-2", firstMap["key"])
	}

	deepList := m["deep_nested"].([]interface{})
	innerList := deepList[0].([]interface{})
	assert.Equal(t, "secret-value-1", innerList[0])
}

func TestLoad_MapTarget(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "default")
	t.Setenv("MODENV_PREFIX", dir)

	targetMap := make(map[string]interface{})
	res, err := modenv.Load(&targetMap)
	assert.NoError(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "modenv-default", targetMap["app_name"])
}

func TestLoad_InvalidTomlSyntax(t *testing.T) {
	tmpDir := t.TempDir()
	invalidToml := `invalid [ toml syntax ===`
	err := os.WriteFile(filepath.Join(tmpDir, ".env.toml"), []byte(invalidToml), 0644)
	assert.NoError(t, err)

	t.Setenv("MODENV_PREFIX", tmpDir)
	_, err = modenv.Load(nil)
	assert.Error(t, err)
}

func TestLoad_InvalidRuntimeTomlSyntax(t *testing.T) {
	tmpDir := t.TempDir()
	validBase := `app_name = "test"`
	invalidRuntime := `invalid = [ syntax`
	err := os.WriteFile(filepath.Join(tmpDir, ".env.toml"), []byte(validBase), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, ".env.production.toml"), []byte(invalidRuntime), 0644)
	assert.NoError(t, err)

	t.Setenv("MODENV_PREFIX", tmpDir)
	t.Setenv("MODENV_RUNTIME", "production")
	_, err = modenv.Load(nil)
	assert.Error(t, err)
}

func TestLoad_InvalidLocalTomlSyntax(t *testing.T) {
	tmpDir := t.TempDir()
	validBase := `app_name = "test"`
	invalidLocal := `invalid = [ local syntax`
	err := os.WriteFile(filepath.Join(tmpDir, ".env.toml"), []byte(validBase), 0644)
	assert.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, ".env.local.toml"), []byte(invalidLocal), 0644)
	assert.NoError(t, err)

	t.Setenv("MODENV_PREFIX", tmpDir)
	_, err = modenv.Load(nil)
	assert.Error(t, err)
}

func TestLoad_NonPointerTargetReturnsError(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "struct")
	t.Setenv("MODENV_PREFIX", dir)

	var cfg Config
	// Passing value instead of pointer returns error
	_, err := modenv.Load(cfg)
	assert.Error(t, err)
}

func TestLoad_InvalidSecretInMap(t *testing.T) {
	tmpDir := t.TempDir()
	tomlContent := `bad_secret = "xor:invalidhex!"`
	err := os.WriteFile(filepath.Join(tmpDir, ".env.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	t.Setenv("MODENV_PREFIX", tmpDir)
	_, err = modenv.Load(nil)
	assert.Error(t, err)
}

func TestLoad_InvalidSecretInSlice(t *testing.T) {
	tmpDir := t.TempDir()
	tomlContent := `bad_secret_slice = ["xor:invalidhex!"]`
	err := os.WriteFile(filepath.Join(tmpDir, ".env.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	t.Setenv("MODENV_PREFIX", tmpDir)
	_, err = modenv.Load(nil)
	assert.Error(t, err)
}

func TestLoad_InvalidSecretInNestedMap(t *testing.T) {
	tmpDir := t.TempDir()
	tomlContent := `
[sub]
bad_secret = "xor:invalidhex!"
`
	err := os.WriteFile(filepath.Join(tmpDir, ".env.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	t.Setenv("MODENV_PREFIX", tmpDir)
	_, err = modenv.Load(nil)
	assert.Error(t, err)
}

func TestLoad_InvalidSecretInSliceOfMaps(t *testing.T) {
	tmpDir := t.TempDir()
	tomlContent := `
[[list_of_maps]]
bad_secret = "xor:invalidhex!"
`
	err := os.WriteFile(filepath.Join(tmpDir, ".env.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	t.Setenv("MODENV_PREFIX", tmpDir)
	_, err = modenv.Load(nil)
	assert.Error(t, err)
}

func TestLoad_InvalidSecretInNestedSlice(t *testing.T) {
	tmpDir := t.TempDir()
	tomlContent := `bad_nested = [["xor:invalidhex!"]]`
	err := os.WriteFile(filepath.Join(tmpDir, ".env.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	t.Setenv("MODENV_PREFIX", tmpDir)
	_, err = modenv.Load(nil)
	assert.Error(t, err)
}

func TestLoad_InvalidSecretInMapSlice(t *testing.T) {
	tmpDir := t.TempDir()
	tomlContent := `
[sub]
bad_slice = ["xor:invalidhex!"]
`
	err := os.WriteFile(filepath.Join(tmpDir, ".env.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	t.Setenv("MODENV_PREFIX", tmpDir)
	_, err = modenv.Load(nil)
	assert.Error(t, err)
}

func TestLoad_WithoutPrefix(t *testing.T) {
	// Temporarily unset MODENV_PREFIX and run from a directory containing .env.toml
	tmpDir := t.TempDir()
	tomlContent := `app_name = "no-prefix-app"`
	err := os.WriteFile(filepath.Join(tmpDir, ".env.toml"), []byte(tomlContent), 0644)
	assert.NoError(t, err)

	origWd, err := os.Getwd()
	assert.NoError(t, err)
	err = os.Chdir(tmpDir)
	assert.NoError(t, err)
	defer func() {
		_ = os.Chdir(origWd)
	}()

	t.Setenv("MODENV_PREFIX", "")
	res, err := modenv.Load(nil)
	assert.NoError(t, err)
	m := res.(map[string]interface{})
	assert.Equal(t, "no-prefix-app", m["app_name"])
}

func TestSimpleSecretEncryptionDecryptionRoundtrip(t *testing.T) {
	plain := "my-secret-value-123"
	encrypted := modenv.EncryptSecret(plain)
	assert.True(t, len(encrypted) > 9)
	assert.True(t, encrypted[:9] == "simple://")

	decrypted, err := modenv.DecryptSecret(encrypted)
	assert.NoError(t, err)
	assert.Equal(t, plain, decrypted)
}

func TestLegacySecretEncryptionDecryption(t *testing.T) {
	plain := "legacy-secret-xyz"
	encrypted := modenv.EncryptLegacySecret(plain)
	assert.True(t, len(encrypted) > 4)
	assert.True(t, encrypted[:4] == "xor:")

	decrypted, err := modenv.DecryptSecret(encrypted)
	assert.NoError(t, err)
	assert.Equal(t, plain, decrypted)
}

func TestPKSSecretEncryptionDecryption(t *testing.T) {
	configsDir := getTestConfigsDir()
	pubKeyBytes, err := os.ReadFile(filepath.Join(configsDir, "test_rsa_public.pem"))
	assert.NoError(t, err)

	plain := "pks-super-secret-password"
	pksCipher, err := modenv.EncryptPKSSecret(plain, string(pubKeyBytes))
	assert.NoError(t, err)
	assert.True(t, len(pksCipher) > 6)
	assert.True(t, pksCipher[:6] == "pks://")

	cfg := modenv.SecretsStoreConfig{
		Type:        "pks",
		KeyLocation: filepath.Join(configsDir, "test_rsa_private.pem"),
	}
	decrypted, err := modenv.DecryptPKSSecret(pksCipher, cfg)
	assert.NoError(t, err)
	assert.Equal(t, plain, decrypted)
}

func TestPKSSecretErrors(t *testing.T) {
	cfg := modenv.SecretsStoreConfig{Type: "pks"}

	// Missing prefix
	_, err := modenv.DecryptPKSSecret("invalid-prefix", cfg)
	assert.Error(t, err)

	// Invalid base64
	_, err = modenv.DecryptPKSSecret("pks://!!!not-b64!!!", cfg)
	assert.Error(t, err)

	// Missing key location
	t.Setenv("MODENV_PRIVATE_KEY", "")
	t.Setenv("MODENV_KEY_LOCATION", "")
	_, err = modenv.DecryptPKSSecret("pks://YWJj", cfg)
	assert.Error(t, err)

	// Invalid key PEM
	t.Setenv("MODENV_PRIVATE_KEY", "not-a-valid-pem")
	_, err = modenv.DecryptPKSSecret("pks://YWJj", cfg)
	assert.Error(t, err)

	// Encrypt with bad public key PEM
	_, err = modenv.EncryptPKSSecret("text", "invalid-pem")
	assert.Error(t, err)
}

func TestCloudSecretResolution(t *testing.T) {
	cfg := modenv.SecretsStoreConfig{
		Type:               "cloud",
		GoogleCloudProject: "test-gcp-proj",
	}

	// 1. Invalid prefix
	_, err := modenv.ResolveCloudSecret("not-cloud://secret", cfg)
	assert.Error(t, err)

	// 2. Missing secret name
	_, err = modenv.ResolveCloudSecret("cloud://", cfg)
	assert.Error(t, err)

	// 3. Env override resolution
	t.Setenv("MODENV_CLOUD_SECRET_DATABASE_PASS", "resolved-env-db-pass")
	res, err := modenv.ResolveCloudSecret("cloud://database-pass", cfg)
	assert.NoError(t, err)
	assert.Equal(t, "resolved-env-db-pass", res)

	// 4. Custom resolver hook
	modenv.SetCloudSecretResolver(func(proj, sec, ver string) (string, error) {
		assert.Equal(t, "test-gcp-proj", proj)
		assert.Equal(t, "custom-secret", sec)
		assert.Equal(t, "latest", ver)
		return "custom-hook-value", nil
	})
	res, err = modenv.ResolveCloudSecret("cloud://custom-secret", cfg)
	assert.NoError(t, err)
	assert.Equal(t, "custom-hook-value", res)

	// Reset custom resolver
	modenv.SetCloudSecretResolver(nil)

	// 5. Full project URI format with custom resolver
	modenv.SetCloudSecretResolver(func(proj, sec, ver string) (string, error) {
		assert.Equal(t, "custom-proj-99", proj)
		assert.Equal(t, "deep-secret", sec)
		assert.Equal(t, "2", ver)
		return "deep-secret-v2", nil
	})
	res, err = modenv.ResolveCloudSecret("cloud://projects/custom-proj-99/secrets/deep-secret/versions/2", cfg)
	assert.NoError(t, err)
	assert.Equal(t, "deep-secret-v2", res)
	modenv.SetCloudSecretResolver(nil)

	// 6. Missing project error when no env or store config
	emptyCfg := modenv.SecretsStoreConfig{Type: "cloud"}
	os.Unsetenv("GOOGLE_CLOUD_PROJECT")
	os.Unsetenv("GCP_PROJECT")
	os.Unsetenv("MODENV_GCP_PROJECT")
	os.Unsetenv("MODENV_CLOUD_SECRET_UNKNOWN_SECRET")
	_, err = modenv.ResolveCloudSecret("cloud://unknown-secret", emptyCfg)
	assert.Error(t, err)
}

func TestLoad_SmartSecretsStore(t *testing.T) {
	configsDir := getTestConfigsDir()
	smartDir := filepath.Join(configsDir, "smart-secrets")
	t.Setenv("MODENV_PREFIX", smartDir)
	t.Setenv("MODENV_KEY_LOCATION", filepath.Join(configsDir, "test_rsa_private.pem"))
	t.Setenv("MODENV_CLOUD_SECRET_MY_DB_SECRET", "cloud-resolved-password-99")
	t.Setenv("MODENV_CLOUD_SECRET_MY_FULL_SECRET", "cloud-full-resolved-token-100")

	res, err := modenv.Load(nil)
	assert.NoError(t, err)

	m := res.(map[string]interface{})
	assert.Equal(t, "smart-secrets-app", m["app_name"])

	db := m["database"].(map[string]interface{})
	assert.Equal(t, "localhost", db["host"])
	assert.Equal(t, "local_db_password", db["simple_password"])
	assert.Equal(t, "local_db_password", db["legacy_password"])
	assert.Equal(t, "pks-decrypted-secret-42", db["pks_password"])
	assert.Equal(t, "cloud-resolved-password-99", db["cloud_password"])
	assert.Equal(t, "cloud-full-resolved-token-100", db["cloud_full_uri"])
}

func TestCloudSecretResolution_HTTPRest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer mock-token-123", r.Header.Get("Authorization"))
		if r.URL.Path == "/v1/projects/my-proj/secrets/db-pass/versions/latest:access" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"name":"...","payload":{"data":"c3VwZXItc2VjcmV0"}}`))
		} else if r.URL.Path == "/v1/projects/my-proj/secrets/bad-b64/versions/latest:access" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"name":"...","payload":{"data":"!!!not-b64!!!"}}`))
		} else if r.URL.Path == "/v1/projects/my-proj/secrets/bad-json/versions/latest:access" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{not valid json`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("MODENV_GCP_ENDPOINT", server.URL)
	t.Setenv("GOOGLE_OAUTH_ACCESS_TOKEN", "mock-token-123")
	cfg := modenv.SecretsStoreConfig{Type: "cloud", GoogleCloudProject: "my-proj"}

	val, err := modenv.ResolveCloudSecret("cloud://db-pass", cfg)
	assert.NoError(t, err)
	assert.Equal(t, "super-secret", val)

	_, err = modenv.ResolveCloudSecret("cloud://non-existent", cfg)
	assert.Error(t, err)

	_, err = modenv.ResolveCloudSecret("cloud://bad-b64", cfg)
	assert.Error(t, err)

	_, err = modenv.ResolveCloudSecret("cloud://bad-json", cfg)
	assert.Error(t, err)
}

func TestCloudSecretResolution_NoCredentials(t *testing.T) {
	t.Setenv("MODENV_GCP_ENDPOINT", "")
	t.Setenv("GOOGLE_OAUTH_ACCESS_TOKEN", "")
	t.Setenv("GCP_ACCESS_TOKEN", "")
	os.Unsetenv("MODENV_CLOUD_SECRET_NO_CREDS_TEST")
	cfg := modenv.SecretsStoreConfig{Type: "cloud", GoogleCloudProject: "my-proj"}
	_, err := modenv.ResolveCloudSecret("cloud://no-creds-test", cfg)
	assert.Error(t, err)
}

func TestPKS_PKCS1Key(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)
	pkcs1Priv := x509.MarshalPKCS1PrivateKey(key)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: pkcs1Priv})

	pkcs1Pub := x509.MarshalPKCS1PublicKey(&key.PublicKey)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: pkcs1Pub})

	enc, err := modenv.EncryptPKSSecret("pkcs1-val", string(pubPEM))
	assert.NoError(t, err)

	t.Setenv("MODENV_PRIVATE_KEY", string(privPEM))
	dec, err := modenv.DecryptPKSSecret(enc, modenv.SecretsStoreConfig{Type: "pks"})
	assert.NoError(t, err)
	assert.Equal(t, "pkcs1-val", dec)
}

func TestPKS_Errors(t *testing.T) {
	_, err := modenv.EncryptPKSSecret("val", "invalid pem block")
	assert.Error(t, err)

	badBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("bad-der")})
	_, err = modenv.EncryptPKSSecret("val", string(badBlock))
	assert.Error(t, err)

	t.Setenv("MODENV_PRIVATE_KEY", "not-a-pem")
	_, err = modenv.DecryptPKSSecret("pks://bad", modenv.SecretsStoreConfig{Type: "pks"})
	assert.Error(t, err)

	badPrivBlock := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: []byte("bad-der")})
	t.Setenv("MODENV_PRIVATE_KEY", string(badPrivBlock))
	_, err = modenv.DecryptPKSSecret("pks://bad", modenv.SecretsStoreConfig{Type: "pks"})
	assert.Error(t, err)

	_, err = modenv.DecryptPKSSecret("pks://invalidbase64@@", modenv.SecretsStoreConfig{
		Type: "pks",
	})
	assert.Error(t, err)
}

func TestPKS_RelativeKeyWithPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)
	pkcs1Priv := x509.MarshalPKCS1PrivateKey(key)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: pkcs1Priv})
	keyPath := filepath.Join(tmpDir, "keys", "private.pem")
	err = os.MkdirAll(filepath.Dir(keyPath), 0755)
	assert.NoError(t, err)
	err = os.WriteFile(keyPath, privPEM, 0600)
	assert.NoError(t, err)

	t.Setenv("MODENV_PREFIX", tmpDir)
	t.Setenv("MODENV_PRIVATE_KEY", "")
	pkcs1Pub := x509.MarshalPKCS1PublicKey(&key.PublicKey)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: pkcs1Pub})
	enc, err := modenv.EncryptPKSSecret("rel-val", string(pubPEM))
	assert.NoError(t, err)

	dec, err := modenv.DecryptPKSSecret(enc, modenv.SecretsStoreConfig{
		Type:        "pks",
		KeyLocation: "keys/private.pem",
	})
	assert.NoError(t, err)
	assert.Equal(t, "rel-val", dec)
}
