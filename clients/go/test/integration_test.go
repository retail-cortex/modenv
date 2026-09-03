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

package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rrmcguinness/modenv/pkg/modenv"
	"github.com/stretchr/testify/assert"
)

func getTestConfigsDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "test/configs" // fallback
	}
	for {
		// Check for test/configs (when run from workspace root)
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
	return "test/configs"
}

func TestIntegrationEnvManager(t *testing.T) {
	manager := modenv.New()

	// Clean slate setup
	_ = os.Unsetenv("INTEGRATION_KEY_1")
	_ = os.Unsetenv("INTEGRATION_KEY_2")

	// Track and set multiple variables
	err := manager.Set("INTEGRATION_KEY_1", "val1")
	assert.NoError(t, err)

	err = manager.Set("INTEGRATION_KEY_2", "val2")
	assert.NoError(t, err)

	// Assert environment updates
	assert.Equal(t, "val1", os.Getenv("INTEGRATION_KEY_1"))
	assert.Equal(t, "val2", os.Getenv("INTEGRATION_KEY_2"))

	// Restore original state
	err = manager.Restore()
	assert.NoError(t, err)

	// Assert environment reverted
	_, ok1 := os.LookupEnv("INTEGRATION_KEY_1")
	_, ok2 := os.LookupEnv("INTEGRATION_KEY_2")
	assert.False(t, ok1)
	assert.False(t, ok2)
}

func TestIntegrationLoad(t *testing.T) {
	dir := filepath.Join(getTestConfigsDir(), "integration")
	t.Setenv("MODENV_PREFIX", dir)

	type IntegrationConfig struct {
		IntegrationVal string `toml:"integration_val"`
		Port           int    `toml:"port"`
	}

	var cfg IntegrationConfig
	cloneCfg, err := modenv.Load(&cfg)
	assert.NoError(t, err)

	assert.Equal(t, "base", cfg.IntegrationVal)
	assert.Equal(t, 2000, cfg.Port) // Overridden by local

	typedClone, ok := cloneCfg.(*IntegrationConfig)
	assert.True(t, ok)
	assert.Equal(t, "base", typedClone.IntegrationVal)
	assert.Equal(t, 2000, typedClone.Port)
}
