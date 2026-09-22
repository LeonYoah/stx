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

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Collection;
import java.util.Collections;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Registry and dispatcher for SeaTunnelEngineAdapter instances. Routes engine calls dynamically
 * based on runtime version and configured policies.
 */
public final class SeaTunnelEngineAdapterRegistry {

    private static final Logger LOG = LoggerFactory.getLogger(SeaTunnelEngineAdapterRegistry.class);

    private static final SeaTunnelEngineAdapterRegistry INSTANCE =
            new SeaTunnelEngineAdapterRegistry();

    private final Map<String, SeaTunnelEngineAdapter> adapters = new ConcurrentHashMap<>();
    private final EngineAdapterVersionPolicy policy;

    private SeaTunnelEngineAdapterRegistry() {
        this.policy = new EngineAdapterVersionPolicy();
        // Register default adapters
        registerAdapter(new SeaTunnel23EngineAdapter());
        registerAdapter(new SeaTunnel30EngineAdapter());
    }

    public static SeaTunnelEngineAdapterRegistry getInstance() {
        return INSTANCE;
    }

    public void registerAdapter(SeaTunnelEngineAdapter adapter) {
        if (adapter != null) {
            adapters.put(adapter.getAdapterVersion(), adapter);
            LOG.info(
                    "Registered SeaTunnel engine adapter for version '{}' ({})",
                    adapter.getAdapterVersion(),
                    adapter.getClass().getSimpleName());
        }
    }

    /** Resolves and returns the best-fit adapter for the given SeaTunnel version string. */
    public SeaTunnelEngineAdapter getAdapter(String versionStr) {
        EngineVersion engineVersion = EngineVersion.parse(versionStr);
        String targetAdapterKey = policy.resolveAdapterVersion(engineVersion);

        SeaTunnelEngineAdapter adapter = adapters.get(targetAdapterKey);
        if (adapter != null) {
            return adapter;
        }

        // Search by adapter.supports(engineVersion)
        for (SeaTunnelEngineAdapter candidate : adapters.values()) {
            if (candidate.supports(engineVersion)) {
                return candidate;
            }
        }

        // Fallback to 2.3 or first available
        if (adapters.containsKey(EngineAdapterVersionPolicy.ADAPTER_V23)) {
            return adapters.get(EngineAdapterVersionPolicy.ADAPTER_V23);
        }
        if (!adapters.isEmpty()) {
            return adapters.values().iterator().next();
        }

        throw new IllegalStateException("No SeaTunnel engine adapters registered");
    }

    public EngineAdapterVersionPolicy getPolicy() {
        return policy;
    }

    public Collection<SeaTunnelEngineAdapter> getAllAdapters() {
        return Collections.unmodifiableCollection(adapters.values());
    }
}
