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

import java.util.Map;
import java.util.Optional;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Tracks changes to environment variables and allows reverting them to their original states.
 */
public class EnvManager {

    private static final EnvManager GLOBAL = new EnvManager();

    private final Map<String, String> overrides = new ConcurrentHashMap<>();
    private final Map<String, Optional<String>> original = new ConcurrentHashMap<>();
    private final Set<String> unsetKeys = ConcurrentHashMap.newKeySet();

    public static EnvManager getInstance() {
        return GLOBAL;
    }

    public EnvManager() {}

    /**
     * Sets an environment variable and records its original value if not already tracked.
     */
    public synchronized void set(String key, String value) {
        if (!original.containsKey(key)) {
            String existing = get(key);
            original.put(key, Optional.ofNullable(existing));
        }
        unsetKeys.remove(key);
        overrides.put(key, value);
    }

    /**
     * Retrieves the value of the environment variable named by key.
     */
    public String get(String key) {
        if (unsetKeys.contains(key)) {
            return null;
        }
        if (overrides.containsKey(key)) {
            return overrides.get(key);
        }
        return System.getenv(key);
    }

    /**
     * Retrieves the value of the environment variable named by key and reports if it was present.
     */
    public synchronized LookupResult lookup(String key) {
        String val = get(key);
        if (val != null) {
            return new LookupResult(val, true);
        }
        return new LookupResult("", false);
    }

    /**
     * Unsets an environment variable and records its original value if not already tracked.
     */
    public synchronized void unset(String key) {
        if (!original.containsKey(key)) {
            String existing = get(key);
            original.put(key, Optional.ofNullable(existing));
        }
        overrides.remove(key);
        unsetKeys.add(key);
    }

    /**
     * Reverts all changes made via this EnvManager to their original values.
     */
    public synchronized void restore() {
        for (Map.Entry<String, Optional<String>> entry : original.entrySet()) {
            String key = entry.getKey();
            Optional<String> origVal = entry.getValue();
            if (origVal.isPresent()) {
                overrides.put(key, origVal.get());
                unsetKeys.remove(key);
            } else {
                overrides.remove(key);
                unsetKeys.remove(key);
            }
        }
        original.clear();
        overrides.clear();
        unsetKeys.clear();
    }

    public static class LookupResult {
        private final String value;
        private final boolean exists;

        public LookupResult(String value, boolean exists) {
            this.value = value;
            this.exists = exists;
        }

        public String getValue() {
            return value;
        }

        public boolean exists() {
            return exists;
        }
    }
}
