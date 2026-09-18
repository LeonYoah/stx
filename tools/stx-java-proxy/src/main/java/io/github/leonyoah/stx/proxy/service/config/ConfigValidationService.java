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

package io.github.leonyoah.stx.proxy.service.config;

import org.apache.seatunnel.shade.com.typesafe.config.Config;
import org.apache.seatunnel.shade.org.apache.commons.lang3.StringUtils;

import org.apache.seatunnel.api.configuration.ReadonlyConfig;

import io.github.leonyoah.stx.proxy.model.JobConfigContext;
import io.github.leonyoah.stx.proxy.model.ProxyNode;
import io.github.leonyoah.stx.proxy.service.plugin.PluginClassLoaderUtils;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.net.URLClassLoader;
import java.sql.Connection;
import java.sql.Driver;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Properties;

import static org.apache.seatunnel.api.options.ConnectorCommonOptions.PLUGIN_NAME;

public class ConfigValidationService {

    private final JobConfigSupportService jobConfigSupportService;

    public ConfigValidationService() {
        this(new JobConfigSupportService());
    }

    ConfigValidationService(JobConfigSupportService jobConfigSupportService) {
        this.jobConfigSupportService = jobConfigSupportService;
    }

    public Map<String, Object> validate(Map<String, Object> request) {
        boolean testConnection = ProxyRequestUtils.getBoolean(request, "testConnection", false);
        JobConfigContext context = jobConfigSupportService.parseJobContext(request);
        List<String> warnings = filterUserVisibleWarnings(context.getWarnings());
        List<String> infos = new ArrayList<>();
        List<String> errors = new ArrayList<>();
        List<Map<String, Object>> checks = new ArrayList<>();

        boolean allChecksSucceeded = true;
        if (testConnection) {
            checks = testConnections(context);
            for (Map<String, Object> check : checks) {
                String status = String.valueOf(check.get("status"));
                if ("failed".equals(status)) {
                    errors.add(
                            String.format(
                                    "%s 连接失败: %s",
                                    check.get("connectorType"), check.get("message")));
                    allChecksSucceeded = false;
                } else if (!"success".equals(status)) {
                    String severity = String.valueOf(check.get("severity"));
                    if ("info".equals(severity)) {
                        infos.add(
                                String.format(
                                        "%s 无需连接测试: %s",
                                        check.get("connectorType"), check.get("message")));
                    } else {
                        warnings.add(
                                String.format(
                                        "%s 暂未完成连接测试: %s",
                                        check.get("connectorType"), check.get("message")));
                        allChecksSucceeded = false;
                    }
                }
            }
        }

        boolean passed = testConnection ? errors.isEmpty() && allChecksSucceeded : errors.isEmpty();
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("ok", passed);
        result.put("valid", passed);
        result.put("errors", errors);
        result.put("warnings", warnings);
        result.put("infos", infos);
        result.put("checks", checks);
        result.put(
                "summary",
                testConnection
                        ? (passed
                                ? "Config validation and connection test passed."
                                : "Config validation or connection test did not pass.")
                        : "Config validation finished.");
        return result;
    }

    private List<Map<String, Object>> testConnections(JobConfigContext context) {
        ClassLoader originalClassLoader = Thread.currentThread().getContextClassLoader();
        URLClassLoader seatunnelHomeClassLoader = null;
        ClassLoader classLoader = originalClassLoader;
        try {
            seatunnelHomeClassLoader =
                    jobConfigSupportService.createSeatunnelHomeClassLoader(originalClassLoader);
            if (seatunnelHomeClassLoader != null) {
                classLoader = seatunnelHomeClassLoader;
                Thread.currentThread().setContextClassLoader(classLoader);
            }

            List<Map<String, Object>> checks = new ArrayList<>();
            for (int i = 0; i < context.getSources().size(); i++) {
                checks.add(
                        testConnectorConnection(
                                context.getGraph().getNodes().get(i),
                                context.getSources().get(i),
                                classLoader));
            }
            int sinkOffset = context.getSources().size() + context.getTransforms().size();
            for (int i = 0; i < context.getSinks().size(); i++) {
                checks.add(
                        testConnectorConnection(
                                context.getGraph().getNodes().get(sinkOffset + i),
                                context.getSinks().get(i),
                                classLoader));
            }
            return checks;
        } finally {
            Thread.currentThread().setContextClassLoader(originalClassLoader);
            PluginClassLoaderUtils.closeQuietly(seatunnelHomeClassLoader);
        }
    }

    private Map<String, Object> testConnectorConnection(
            ProxyNode node, Config config, ClassLoader classLoader) {
        ReadonlyConfig readonlyConfig = ReadonlyConfig.fromConfig(config);
        String pluginName = readonlyConfig.get(PLUGIN_NAME);
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("nodeId", node.getNodeId());
        result.put("kind", node.getKind().name().toLowerCase());
        result.put(
                "connectorType",
                String.format(
                        "%s[%d]-%s",
                        StringUtils.capitalize(node.getKind().name().toLowerCase()),
                        node.getConfigIndex(),
                        pluginName));
        result.put("target", StringUtils.defaultString(getString(config, "url")));

        if (doesNotRequireConnectionProbe(pluginName)) {
            result.put("status", "skipped");
            result.put("severity", "info");
            result.put("message", "该连接器不依赖外部服务，无需连接测试。");
            return result;
        }

        if (!"Jdbc".equalsIgnoreCase(pluginName)) {
            result.put("status", "skipped");
            result.put("severity", "warning");
            result.put("message", "该连接器暂不支持连接探测，请通过配置预览或实际运行进一步验证。");
            return result;
        }

        try {
            probeJdbc(config, classLoader);
            result.put("status", "success");
            result.put("message", "Connection succeeded.");
            return result;
        } catch (Exception e) {
            result.put("status", "failed");
            result.put("message", firstLine(e.getMessage()));
            return result;
        }
    }

    /**
     * 过滤仅用于内部调试的兼容提示，避免在连接测试结果里误导用户。 Filters internal compatibility hints so connection test
     * results do not confuse users.
     */
    private List<String> filterUserVisibleWarnings(List<String> warnings) {
        List<String> result = new ArrayList<>();
        for (String warning : warnings) {
            if (StringUtils.startsWith(warning, "Simple graph compatibility linked ")) {
                continue;
            }
            result.add(warning);
        }
        return result;
    }

    /**
     * 判断连接器是否天然不需要外部连接探测。 Detects connectors that naturally do not need external connection
     * probing.
     */
    private boolean doesNotRequireConnectionProbe(String pluginName) {
        return "FakeSource".equalsIgnoreCase(pluginName) || "Console".equalsIgnoreCase(pluginName);
    }

    private void probeJdbc(Config config, ClassLoader classLoader) throws Exception {
        String url = getRequiredString(config, "url");
        String username = getString(config, "username");
        String password = getString(config, "password");
        String driverClassName = getString(config, "driver");
        if (StringUtils.isNotBlank(driverClassName)) {
            Class<?> driverClass = Class.forName(driverClassName, true, classLoader);
            Driver driver = (Driver) driverClass.getDeclaredConstructor().newInstance();
            Properties properties = new Properties();
            if (StringUtils.isNotBlank(username)) {
                properties.setProperty("user", username);
            }
            if (password != null) {
                properties.setProperty("password", password);
            }
            try (Connection ignored = driver.connect(url, properties)) {
                if (ignored == null) {
                    throw new IllegalStateException("JDBC driver returned null connection.");
                }
            }
            return;
        }
        throw new IllegalArgumentException("Missing jdbc driver option.");
    }

    private String getRequiredString(Config config, String path) {
        String value = getString(config, path);
        if (StringUtils.isBlank(value)) {
            throw new IllegalArgumentException("Missing required option: " + path);
        }
        return value;
    }

    private String getString(Config config, String path) {
        return config != null && config.hasPath(path) ? config.getString(path) : null;
    }

    private String firstLine(String message) {
        if (StringUtils.isBlank(message)) {
            return "unknown error";
        }
        int newline = message.indexOf('\n');
        if (newline < 0) {
            return message;
        }
        return message.substring(0, newline).trim();
    }
}
