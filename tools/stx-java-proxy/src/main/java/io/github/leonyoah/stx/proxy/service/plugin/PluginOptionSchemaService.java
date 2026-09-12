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

package io.github.leonyoah.stx.proxy.service.plugin;

import org.apache.seatunnel.api.table.factory.Factory;

import io.github.leonyoah.stx.proxy.model.PluginOptionDescriptor;
import io.github.leonyoah.stx.proxy.model.PluginOptionSchemaResult;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.util.ArrayList;
import java.util.Collections;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

public class PluginOptionSchemaService {

    private static final Map<String, PluginOptionSchemaResult> SCHEMA_CACHE =
            new ConcurrentHashMap<>();

    private final PluginRuntimeService pluginRuntimeService;
    private final FactoryOptionRuleExtractor optionRuleExtractor;
    private final OptionFieldScanService optionFieldScanService;

    public PluginOptionSchemaService() {
        this(
                new PluginRuntimeService(),
                new FactoryOptionRuleExtractor(),
                new OptionFieldScanService());
    }

    PluginOptionSchemaService(
            PluginRuntimeService pluginRuntimeService,
            FactoryOptionRuleExtractor optionRuleExtractor,
            OptionFieldScanService optionFieldScanService) {
        this.pluginRuntimeService = pluginRuntimeService;
        this.optionRuleExtractor = optionRuleExtractor;
        this.optionFieldScanService = optionFieldScanService;
    }

    public PluginOptionSchemaResult inspect(Map<String, Object> request) {
        PluginRuntimeService.PluginExecutionContext context =
                pluginRuntimeService.openContext(request);
        Thread currentThread = Thread.currentThread();
        ClassLoader originalClassLoader = currentThread.getContextClassLoader();
        try {
            currentThread.setContextClassLoader(context.getClassLoader());
            String factoryIdentifier =
                    ProxyRequestUtils.getRequiredString(request, "factoryIdentifier");
            boolean includeSupplement =
                    ProxyRequestUtils.getBoolean(request, "includeSupplement", true);
            String cacheKey =
                    context.getPluginType()
                            + "|"
                            + factoryIdentifier.toLowerCase()
                            + "|"
                            + context.getClasspathFingerprint()
                            + "|"
                            + includeSupplement;
            PluginOptionSchemaResult cached = SCHEMA_CACHE.get(cacheKey);
            if (cached != null) {
                return cached;
            }
            Factory factory = pluginRuntimeService.discoverFactory(context, factoryIdentifier);
            FactoryOptionRuleExtractor.ExtractionResult optionRuleResult =
                    optionRuleExtractor.extract(context.getPluginType(), factory);
            Map<String, PluginOptionDescriptor> merged = new LinkedHashMap<>();
            for (PluginOptionDescriptor option : optionRuleResult.getOptions()) {
                merged.put(option.getKey(), option);
            }
            List<String> warnings = new ArrayList<>(optionRuleResult.getWarnings());
            if (includeSupplement) {
                OptionFieldScanService.ScanResult fieldScanResult =
                        optionFieldScanService.scan(factory, context, pluginRuntimeService);
                warnings.addAll(fieldScanResult.getWarnings());
                for (PluginOptionDescriptor option : fieldScanResult.getOptions()) {
                    PluginOptionDescriptor existing = merged.get(option.getKey());
                    if (existing == null) {
                        merged.put(option.getKey(), option);
                    } else {
                        PluginOptionSupport.merge(existing, option);
                    }
                }
            }
            List<PluginOptionDescriptor> options = new ArrayList<>(merged.values());
            options.sort(
                    Comparator.comparingInt(
                                    (PluginOptionDescriptor item) -> weight(item.getRequiredMode()))
                            .thenComparing(
                                    PluginOptionDescriptor::getKey, String.CASE_INSENSITIVE_ORDER));
            PluginOptionSchemaResult result =
                    new PluginOptionSchemaResult(
                            true,
                            context.getPluginType(),
                            factory.factoryIdentifier(),
                            Collections.unmodifiableList(options),
                            Collections.unmodifiableList(warnings));
            SCHEMA_CACHE.put(cacheKey, result);
            return result;
        } finally {
            currentThread.setContextClassLoader(originalClassLoader);
            context.close();
        }
    }

    private int weight(io.github.leonyoah.stx.proxy.model.RequiredMode mode) {
        if (mode == null) {
            return 99;
        }
        switch (mode) {
            case REQUIRED:
                return 0;
            case EXCLUSIVE:
            case CONDITIONAL:
            case BUNDLED:
                return 1;
            case OPTIONAL:
                return 2;
            case SUPPLEMENTAL_OPTIONAL:
                return 3;
            case UNKNOWN_NO_DEFAULT:
                return 4;
            default:
                return 99;
        }
    }
}
