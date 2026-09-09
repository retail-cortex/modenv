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
	code := run([]string{"modenv", "help"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "Usage: modenv")

	stdout.Reset()
	code = run([]string{"modenv", "-h"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "Usage: modenv")

	stdout.Reset()
	code = run([]string{"modenv", "--help"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "Usage: modenv")
}

func TestCliNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 1, code)
}

func TestCliUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv", "unknown-cmd"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Unknown command")
}

func TestCliEncode_PositionalArgumentDisabled(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv", "encode", "my-secret"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Passing secret values directly on the command line is disabled")
	assert.Contains(t, stderr.String(), "shell history")
}

func TestCliEncode_Simple(t *testing.T) {
	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("my-secret\nmy-secret\n")
	code := run([]string{"modenv", "encode"}, stdin, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr.String(), "Enter secret: ")
	assert.Contains(t, stderr.String(), "Confirm secret: ")

	out := strings.TrimSpace(stdout.String())
	assert.True(t, strings.HasPrefix(out, "simple://"))
	decrypted, err := modenv.DecryptSecret(out)
	assert.NoError(t, err)
	assert.Equal(t, "my-secret", decrypted)

	// Test with --encode alias and --type=simple flag
	stdout.Reset()
	stderr.Reset()
	stdin = strings.NewReader("my-secret-2\nmy-secret-2\n")
	code = run([]string{"modenv", "--encode", "--type=simple"}, stdin, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr.String(), "Enter secret: ")
	assert.Contains(t, stderr.String(), "Confirm secret: ")

	out = strings.TrimSpace(stdout.String())
	assert.True(t, strings.HasPrefix(out, "simple://"))
	decrypted, err = modenv.DecryptSecret(out)
	assert.NoError(t, err)
	assert.Equal(t, "my-secret-2", decrypted)
}

func TestCliEncode_Legacy(t *testing.T) {
	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("legacy-val\nlegacy-val\n")
	code := run([]string{"modenv", "encode", "--legacy"}, stdin, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr.String(), "Enter secret: ")
	assert.Contains(t, stderr.String(), "Confirm secret: ")

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
	stdin := strings.NewReader("super-secret-pks\nsuper-secret-pks\n")
	code := run([]string{"modenv", "encode", "--type=pks", "--public-key=" + pubPath}, stdin, &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr.String(), "Enter secret: ")
	assert.Contains(t, stderr.String(), "Confirm secret: ")

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
	stdin = strings.NewReader("another-pks\nanother-pks\n")
	code = run([]string{"modenv", "encode", "-t", "pks", "-k", pubPath}, stdin, &stdout, &stderr)
	assert.Equal(t, 0, code)
	out = strings.TrimSpace(stdout.String())
	assert.True(t, strings.HasPrefix(out, "pks://"))

	// Test with MODENV_PUBLIC_KEY env var
	t.Setenv("MODENV_PUBLIC_KEY", string(pubPEM))
	stdout.Reset()
	stderr.Reset()
	stdin = strings.NewReader("env-pub-key-val\nenv-pub-key-val\n")
	code = run([]string{"modenv", "encode", "--type=pks"}, stdin, &stdout, &stderr)
	assert.Equal(t, 0, code)
	out = strings.TrimSpace(stdout.String())
	assert.True(t, strings.HasPrefix(out, "pks://"))
	t.Setenv("MODENV_PUBLIC_KEY", "")

	// Test --type and --public-key with spaces
	stdout.Reset()
	stderr.Reset()
	stdin = strings.NewReader("spaced-pks\nspaced-pks\n")
	code = run([]string{"modenv", "encode", "--type", "pks", "--public-key", pubPath}, stdin, &stdout, &stderr)
	assert.Equal(t, 0, code)
	out = strings.TrimSpace(stdout.String())
	assert.True(t, strings.HasPrefix(out, "pks://"))
}

func TestCliEncode_PromptValidationErrors(t *testing.T) {
	// Secret mismatch
	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("secret-one\nsecret-different\n")
	code := run([]string{"modenv", "encode"}, stdin, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Secrets do not match")

	// Empty secret
	stdout.Reset()
	stderr.Reset()
	stdin = strings.NewReader("\n\n")
	code = run([]string{"modenv", "encode"}, stdin, &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Secret cannot be empty")
}

func TestCliEncode_FlagErrors(t *testing.T) {
	// PKS without public key
	t.Setenv("MODENV_PUBLIC_KEY", "")
	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv", "encode", "--type=pks"}, strings.NewReader("sec\nsec\n"), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "PKS encryption requires")

	// PKS with non-existent key file
	stderr.Reset()
	code = run([]string{"modenv", "encode", "--type=pks", "--public-key=/no/such/file.pem"}, strings.NewReader("sec\nsec\n"), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Error reading public key")

	// Unknown type
	stderr.Reset()
	code = run([]string{"modenv", "encode", "--type=invalid-type"}, strings.NewReader("sec\nsec\n"), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "Unknown encryption type")
}

func TestCliSetupAndRead(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MODENV_PREFIX", tmpDir)

	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv", "setup"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.FileExists(t, filepath.Join(tmpDir, ".env.toml"))
	assert.FileExists(t, filepath.Join(tmpDir, ".env.local.toml"))

	// Second setup skips existing files
	stdout.Reset()
	code = run([]string{"modenv", "setup"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "Skipping")

	// Read config
	stdout.Reset()
	code = run([]string{"modenv", "read"}, strings.NewReader(""), &stdout, &stderr)
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
	code := run([]string{"modenv", "read"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "is missing")
}

func TestCliReadInvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MODENV_PREFIX", tmpDir)
	err := os.WriteFile(filepath.Join(tmpDir, ".env.toml"), []byte("invalid = [ syntax"), 0644)
	assert.NoError(t, err)

	var stdout, stderr bytes.Buffer
	code := run([]string{"modenv", "read"}, strings.NewReader(""), &stdout, &stderr)
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
	code := run([]string{"modenv", "read"}, strings.NewReader(""), &stdout, &stderr)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout.String(), "(working directory)")
	assert.Contains(t, stdout.String(), "(none)")
}
