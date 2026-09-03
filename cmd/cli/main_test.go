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

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rrmcguinness/modenv/pkg/modenv"
	"github.com/stretchr/testify/assert"
)

func TestCliHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv", "help"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "Usage: modenv")

	stdout.Reset()
	code = run([]string{"modenv", "-h"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "Usage: modenv")

	stdout.Reset()
	code = run([]string{"modenv", "--help"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "Usage: modenv")
}

func TestCliNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv"}, &stdout, &stderr)
	assert.Equal(t, 1, code)
}

func TestCliUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv", "unknown-cmd"}, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Unknown command")
}

func TestCliEncode_Simple(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv", "encode", "my-secret"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	out := strings.TrimSpace(stdout.String())
	assert.True(t, strings.HasPrefix(out, "simple://"))
	decrypted, err := modenv.DecryptSecret(out)
	assert.NoError(t, err)
	assert.Equal(t, "my-secret", decrypted)

	stdout.Reset()
	code = run([]string{"modenv", "--encode", "--type=simple", "my-secret-2"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	out = strings.TrimSpace(stdout.String())
	assert.True(t, strings.HasPrefix(out, "simple://"))
	decrypted, err = modenv.DecryptSecret(out)
	assert.NoError(t, err)
	assert.Equal(t, "my-secret-2", decrypted)
}

func TestCliEncode_Legacy(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv", "encode", "--legacy", "legacy-val"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	out := strings.TrimSpace(stdout.String())
	assert.True(t, strings.HasPrefix(out, "xor:"))
	decrypted, err := modenv.DecryptSecret(out)
	assert.NoError(t, err)
	assert.Equal(t, "legacy-val", decrypted)
}

func TestCliEncode_PKS(t *testing.T) {
	tmpDir := t.TempDir()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)

	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	assert.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	pubPath := filepath.Join(tmpDir, "pub.pem")
	err = os.WriteFile(pubPath, pubPEM, 0644)
	assert.NoError(t, err)

	privDER := x509.MarshalPKCS1PrivateKey(key)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER})

	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv", "encode", "--type=pks", "--public-key=" + pubPath, "super-secret-pks"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	out := strings.TrimSpace(stdout.String())
	assert.True(t, strings.HasPrefix(out, "pks://"))

	// Decrypt using modenv with key
	t.Setenv("MODENV_PRIVATE_KEY", string(privPEM))
	decrypted, err := modenv.DecryptPKSSecret(out, modenv.SecretsStoreConfig{Type: "pks"})
	assert.NoError(t, err)
	assert.Equal(t, "super-secret-pks", decrypted)

	// Test short flags -t pks -k <path>
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"modenv", "encode", "-t", "pks", "-k", pubPath, "another-pks"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	out = strings.TrimSpace(stdout.String())
	assert.True(t, strings.HasPrefix(out, "pks://"))

	// Test with MODENV_PUBLIC_KEY env var
	t.Setenv("MODENV_PUBLIC_KEY", string(pubPEM))
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"modenv", "encode", "--type=pks", "env-pub-key-val"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	out = strings.TrimSpace(stdout.String())
	assert.True(t, strings.HasPrefix(out, "pks://"))
	t.Setenv("MODENV_PUBLIC_KEY", "")

	// Test --type and --public-key with spaces
	stdout.Reset()
	stderr.Reset()
	code = run([]string{"modenv", "encode", "--type", "pks", "--public-key", pubPath, "spaced-pks"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	out = strings.TrimSpace(stdout.String())
	assert.True(t, strings.HasPrefix(out, "pks://"))
}

func TestCliEncode_Errors(t *testing.T) {
	// Missing secret
	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv", "encode"}, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Missing secret to encode")

	// PKS without public key
	t.Setenv("MODENV_PUBLIC_KEY", "")
	stderr.Reset()
	code = run([]string{"modenv", "encode", "--type=pks", "secret"}, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "PKS encryption requires")

	// PKS with non-existent key file
	stderr.Reset()
	code = run([]string{"modenv", "encode", "--type=pks", "--public-key=/no/such/file.pem", "secret"}, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Error reading public key")

	// PKS with invalid PEM content
	tmpDir := t.TempDir()
	badPEMPath := filepath.Join(tmpDir, "bad.pem")
	_ = os.WriteFile(badPEMPath, []byte("bad content"), 0644)
	stderr.Reset()
	code = run([]string{"modenv", "encode", "--type=pks", "--public-key=" + badPEMPath, "secret"}, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Error encrypting secret")

	// Unknown type
	stderr.Reset()
	code = run([]string{"modenv", "encode", "--type=invalid-type", "secret"}, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Unknown encryption type")
}

func TestCliSetupAndRead(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MODENV_PREFIX", tmpDir)

	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv", "setup"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.FileExists(t, filepath.Join(tmpDir, ".env.toml"))
	assert.FileExists(t, filepath.Join(tmpDir, ".env.local.toml"))

	// Second setup skips existing files
	stdout.Reset()
	code = run([]string{"modenv", "setup"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "Skipping")

	// Read config
	stdout.Reset()
	code = run([]string{"modenv", "read"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "Resolved Configuration Tree")
	assert.Contains(t, stdout.String(), `app_name = "my-app"`)
	// Secret should be decrypted
	assert.Contains(t, stdout.String(), "local_db_password")
}

func TestCliReadMissingBaseFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MODENV_PREFIX", tmpDir)

	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv", "read"}, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "is missing")
}

func TestCliReadInvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MODENV_PREFIX", tmpDir)
	err := os.WriteFile(filepath.Join(tmpDir, ".env.toml"), []byte("invalid = [ syntax"), 0644)
	assert.NoError(t, err)

	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv", "read"}, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Error loading configuration")
}

func TestCliReadWithoutPrefixOrRuntime(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, err := os.Getwd()
	assert.NoError(t, err)
	err = os.Chdir(tmpDir)
	assert.NoError(t, err)
	defer func() {
		_ = os.Chdir(origWd)
	}()

	err = os.WriteFile(filepath.Join(tmpDir, ".env.toml"), []byte(`app_name = "test-no-prefix"`), 0644)
	assert.NoError(t, err)

	t.Setenv("MODENV_PREFIX", "")
	t.Setenv("MODENV_RUNTIME", "")

	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv", "read"}, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "(working directory)")
	assert.Contains(t, stdout.String(), "(none)")
}
