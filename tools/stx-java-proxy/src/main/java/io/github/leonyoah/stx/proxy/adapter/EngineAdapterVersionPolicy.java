/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package io.github.leonyoah.stx.proxy.adapter;

import org.apache.seatunnel.shade.org.apache.commons.lang3.StringUtils;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Manages version-to-adapter resolution with configurable mappings and forward/backward fallback
 * policies. Enables zero-code reuse of adapters when minor engine releases do not introduce
 * breaking API changes.
 */
public final class EngineAdapterVersionPolicy {

    private static final Logger LOG = LoggerFactory.getLogger(EngineAdapterVersionPolicy.class);

    public static final String PROPERTY_VERSION_MAPPING = "stx.adapter.version.mapping";
    public static final String ENV_VERSION_MAPPING = "STX_ADAPTER_VERSION_MAPPING";
    public static final String PROPERTY_DEFAULT_FALLBACK = "stx.adapter.version.fallback";
    public static final String ENV_DEFAULT_FALLBACK = "STX_ADAPTER_VERSION_FALLBACK";

    public static final String ADAPTER_V23 = "2.3";
    public static final String ADAPTER_V30 = "3.0";

    private final Map<String, String> dynamicMappings = new ConcurrentHashMap<>();
    private volatile String defaultFallbackAdapter = ADAPTER_V23;

    public EngineAdapterVersionPolicy() {
        loadConfiguredMappings();
    }

    /**
     * Resolves target adapter identifier for a given EngineVersion. Order of evaluation: 1. Dynamic
     * configured mappings (system properties & env vars) 2. Built-in version rules (exact, prefix,
     * wildcard) 3. Major version forward compatibility (e.g. 3.x -> 3.0, 2.x -> 2.3) 4. Configured
     * fallback
     */
    public String resolveAdapterVersion(EngineVersion version) {
        if (version == null) {
            return defaultFallbackAdapter;
        }

        // 1. Dynamic configured mappings
        for (Map.Entry<String, String> entry : dynamicMappings.entrySet()) {
            if (version.matches(entry.getKey())) {
                LOG.info(
                        "Resolved SeaTunnel version '{}' to adapter '{}' via configured mapping rule '{}'",
                        version,
                        entry.getValue(),
                        entry.getKey());
                return entry.getValue();
            }
        }

        // 2. Built-in exact and minor matching
        if (version.matches("3.0.*")) {
            return ADAPTER_V30;
        }
        if (version.matches("2.3.*")) {
            return ADAPTER_V23;
        }

        // 3. Forward compatibility fallback for 3.x series
        if (version.isVersion3x()) {
            LOG.info(
                    "Resolved SeaTunnel version '{}' to adapter '{}' via 3.x forward-compatible rule",
                    version,
                    ADAPTER_V30);
            return ADAPTER_V30;
        }

        // Backward compatibility fallback for 2.x series
        if (version.isVersion2x()) {
            LOG.info(
                    "Resolved SeaTunnel version '{}' to adapter '{}' via 2.x backward-compatible rule",
                    version,
                    ADAPTER_V23);
            return ADAPTER_V23;
        }

        LOG.warn(
                "SeaTunnel version '{}' has no specific adapter match. Using fallback adapter '{}'",
                version,
                defaultFallbackAdapter);
        return defaultFallbackAdapter;
    }

    public void registerMapping(String versionPattern, String targetAdapterVersion) {
        if (StringUtils.isNotBlank(versionPattern)
                && StringUtils.isNotBlank(targetAdapterVersion)) {
            dynamicMappings.put(versionPattern.trim(), targetAdapterVersion.trim());
        }
    }

    public void setDefaultFallbackAdapter(String fallback) {
        if (StringUtils.isNotBlank(fallback)) {
            this.defaultFallbackAdapter = fallback.trim();
        }
    }

    public Map<String, String> getMappings() {
        return Collections.unmodifiableMap(new LinkedHashMap<>(dynamicMappings));
    }

    private void loadConfiguredMappings() {
        String fallback = System.getProperty(PROPERTY_DEFAULT_FALLBACK);
        if (StringUtils.isBlank(fallback)) {
            fallback = System.getenv(ENV_DEFAULT_FALLBACK);
        }
        if (StringUtils.isNotBlank(fallback)) {
            this.defaultFallbackAdapter = fallback.trim();
        }

        String rawMappings = System.getProperty(PROPERTY_VERSION_MAPPING);
        if (StringUtils.isBlank(rawMappings)) {
            rawMappings = System.getenv(ENV_VERSION_MAPPING);
        }
        if (StringUtils.isNotBlank(rawMappings)) {
            // Semicolon or comma separated list: "3.1.*:3.0;3.2.*:3.0"
            String[] pairs = rawMappings.split("[;,]");
            for (String pair : pairs) {
                String trimmed = pair.trim();
                if (trimmed.contains(":") || trimmed.contains("=")) {
                    String[] kv = trimmed.split("[:=]", 2);
                    if (kv.length == 2
                            && StringUtils.isNotBlank(kv[0])
                            && StringUtils.isNotBlank(kv[1])) {
                        registerMapping(kv[0].trim(), kv[1].trim());
                    }
                }
            }
        }
    }
}
