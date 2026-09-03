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

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;

import static org.junit.jupiter.api.Assertions.*;

public class IntegrationTest {

    public static class IntegrationConfig {
        public String integrationVal;
        public int port;
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
    public void testIntegrationEnvManager() {
        EnvManager manager = new EnvManager();
        manager.unset("INTEGRATION_KEY_1");
        manager.unset("INTEGRATION_KEY_2");

        manager.set("INTEGRATION_KEY_1", "val1");
        manager.set("INTEGRATION_KEY_2", "val2");

        assertEquals("val1", manager.get("INTEGRATION_KEY_1"));
        assertEquals("val2", manager.get("INTEGRATION_KEY_2"));

        manager.restore();

        assertFalse(manager.lookup("INTEGRATION_KEY_1").exists());
        assertFalse(manager.lookup("INTEGRATION_KEY_2").exists());
    }

    @Test
    public void testIntegrationLoad() throws IOException {
        Path dir = getTestConfigsDir().resolve("integration");
        EnvManager.getInstance().set("MODENV_PREFIX", dir.toString());

        IntegrationConfig cfg = new IntegrationConfig();
        IntegrationConfig clone = Modenv.load(cfg);

        assertEquals("base", cfg.integrationVal);
        assertEquals(2000, cfg.port);

        assertEquals("base", clone.integrationVal);
        assertEquals(2000, clone.port);
    }
}
