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

package io.github.leonyoah.stx.proxy.service.storage;

import org.apache.seatunnel.shade.com.typesafe.config.Config;
import org.apache.seatunnel.shade.com.typesafe.config.ConfigException;
import org.apache.seatunnel.shade.com.typesafe.config.ConfigFactory;
import org.apache.seatunnel.shade.com.typesafe.config.ConfigParseOptions;
import org.apache.seatunnel.shade.com.typesafe.config.ConfigResolveOptions;
import org.apache.seatunnel.shade.com.typesafe.config.ConfigSyntax;
import org.apache.seatunnel.shade.com.typesafe.config.impl.Parseable;
import org.apache.seatunnel.shade.org.apache.commons.lang3.StringUtils;

import io.github.leonyoah.stx.proxy.service.support.ProxyException;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Properties;
import java.util.Set;

public class CheckpointSourceActionMatcher {

    public static final class SourceTarget {
        private final int configIndex;
        private final String pluginName;
        private final String actionName;

        SourceTarget(int configIndex, String pluginName, String actionName) {
            this.configIndex = configIndex;
            this.pluginName = pluginName;
            this.actionName = actionName;
        }

        public int getConfigIndex() {
            return configIndex;
        }

        public String getPluginName() {
            return pluginName;
        }

        public String getActionName() {
            return actionName;
        }
    }

    public List<SourceTarget> match(Map<String, Object> request) {
        Map<String, Object> jobConfigRequest = ProxyRequestUtils.getMap(request, "jobConfig");
        if (jobConfigRequest.isEmpty()) {
            throw new ProxyException(400, "inspect-source-state requires jobConfig");
        }
        Config config = loadJobConfig(jobConfigRequest);
        List<Config> sources = getConfigList(config, "source");
        List<Integer> requestedTargets = getIntegerList(request, "sourceTargets");
        Set<Integer> selectedTargets =
                requestedTargets.isEmpty() ? null : new LinkedHashSet<>(requestedTargets);

        List<SourceTarget> results = new ArrayList<>();
        for (int index = 0; index < sources.size(); index++) {
            if (selectedTargets != null && !selectedTargets.contains(index)) {
                continue;
            }
            Config source = sources.get(index);
            String pluginName = source.getString("plugin_name");
            results.add(new SourceTarget(index, pluginName, buildActionName(index, pluginName)));
        }
        return results;
    }

    static String buildActionName(int sourceIndex, String pluginName) {
        return "Source[" + sourceIndex + "]-" + pluginName;
    }

    private Config loadJobConfig(Map<String, Object> request) {
        String content = ProxyRequestUtils.getOptionalString(request, "content");
        String contentFormat = ProxyRequestUtils.getOptionalString(request, "contentFormat");
        String filePath = ProxyRequestUtils.getOptionalString(request, "filePath");
        if (StringUtils.isNotBlank(filePath)) {
            throw new ProxyException(
                    400, "inspect-source-state does not support jobConfig.filePath");
        }
        if (StringUtils.isBlank(content)) {
            throw new ProxyException(400, "jobConfig.content is required");
        }
        return parseConfigContent(
                content, extractVariables(request.get("variables")), contentFormat);
    }

    private Config parseConfigContent(
            String content, Map<String, String> variables, String contentFormat) {
        try {
            ConfigResolveOptions resolveOptions =
                    ConfigResolveOptions.defaults().setAllowUnresolved(true);
            ConfigParseOptions parseOptions = buildParseOptions(contentFormat);
            Config config =
                    ConfigFactory.parseString(content, parseOptions).resolve(resolveOptions);
            if (!variables.isEmpty()) {
                Properties properties = new Properties();
                for (Map.Entry<String, String> entry : variables.entrySet()) {
                    properties.setProperty(entry.getKey(), entry.getValue());
                }
                Config variableConfig =
                        Parseable.newProperties(
                                        properties,
                                        ConfigParseOptions.defaults()
                                                .setOriginDescription("proxy variables"))
                                .parse()
                                .toConfig();
                config = config.resolveWith(variableConfig, resolveOptions);
            }
            return config.resolveWith(ConfigFactory.systemProperties(), resolveOptions);
        } catch (ConfigException e) {
            throw new ProxyException(400, "jobConfig parse failed: " + e.getMessage(), e);
        }
    }

    private ConfigParseOptions buildParseOptions(String contentFormat) {
        if (StringUtils.isBlank(contentFormat) || "hocon".equalsIgnoreCase(contentFormat)) {
            return ConfigParseOptions.defaults();
        }
        if ("json".equalsIgnoreCase(contentFormat)) {
            return ConfigParseOptions.defaults().setSyntax(ConfigSyntax.JSON);
        }
        throw new ProxyException(
                400,
                "Unsupported jobConfig.contentFormat: "
                        + contentFormat
                        + ", expected hocon or json");
    }

    private List<Config> getConfigList(Config jobConfig, String path) {
        if (!jobConfig.hasPath(path)) {
            return Collections.emptyList();
        }
        return new ArrayList<>(jobConfig.getConfigList(path));
    }

    private Map<String, String> extractVariables(Object rawVariables) {
        if (rawVariables == null) {
            return Collections.emptyMap();
        }
        if (rawVariables instanceof Map) {
            Map<String, String> result = new LinkedHashMap<>();
            @SuppressWarnings("unchecked")
            Map<String, Object> rawMap = (Map<String, Object>) rawVariables;
            for (Map.Entry<String, Object> entry : rawMap.entrySet()) {
                result.put(
                        entry.getKey(),
                        entry.getValue() == null ? "" : String.valueOf(entry.getValue()));
            }
            return result;
        }
        if (rawVariables instanceof List) {
            Map<String, String> result = new LinkedHashMap<>();
            @SuppressWarnings("unchecked")
            List<Object> items = (List<Object>) rawVariables;
            for (Object item : items) {
                if (item == null) {
                    continue;
                }
                String variable = String.valueOf(item);
                String[] pair = variable.split("=", 2);
                if (pair.length != 2 || StringUtils.isBlank(pair[0])) {
                    throw new ProxyException(
                            400, "Invalid variable, expected key=value: " + variable);
                }
                result.put(pair[0], pair[1]);
            }
            return result;
        }
        throw new ProxyException(400, "jobConfig.variables must be an object or array");
    }

    private List<Integer> getIntegerList(Map<String, Object> request, String key) {
        if (request == null || !request.containsKey(key) || request.get(key) == null) {
            return Collections.emptyList();
        }
        Object value = request.get(key);
        if (!(value instanceof List)) {
            throw new ProxyException(400, "Field '" + key + "' must be an array");
        }
        @SuppressWarnings("unchecked")
        List<Object> rawList = (List<Object>) value;
        List<Integer> result = new ArrayList<>(rawList.size());
        for (Object item : rawList) {
            if (item instanceof Number) {
                result.add(((Number) item).intValue());
            } else {
                result.add(Integer.parseInt(String.valueOf(item)));
            }
        }
        return result;
    }
}
