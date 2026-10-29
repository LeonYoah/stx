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

import io.github.leonyoah.stx.proxy.model.JobConfigContext;
import io.github.leonyoah.stx.proxy.model.ProxyNode;
import io.github.leonyoah.stx.proxy.service.plugin.PluginClassLoaderUtils;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.net.URLClassLoader;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

public class ConfigValidationService {

    private static final ExecutorService PROBE_EXECUTOR =
            Executors.newCachedThreadPool(
                    r -> {
                        Thread t = new Thread(r, "stx-connection-probe");
                        t.setDaemon(true);
                        return t;
                    });

    private final JobConfigSupportService jobConfigSupportService;
    private final OptionRuleValidationService optionRuleValidationService;

    public ConfigValidationService() {
        this(new JobConfigSupportService(), new OptionRuleValidationService());
    }

    /**
     * 测试用：仅注入 JobConfigSupport；不跑 OptionRule（避免 Fake 上下文误伤连接测试用例）。
     *
     * <p>Test helper: inject JobConfigSupport only; skip OptionRule to avoid breaking
     * connection-test fakes.
     */
    ConfigValidationService(JobConfigSupportService jobConfigSupportService) {
        this(jobConfigSupportService, null);
    }

    ConfigValidationService(
            JobConfigSupportService jobConfigSupportService,
            OptionRuleValidationService optionRuleValidationService) {
        this.jobConfigSupportService = jobConfigSupportService;
        this.optionRuleValidationService = optionRuleValidationService;
    }

    public Map<String, Object> validate(Map<String, Object> request) {
        boolean testConnection = ProxyRequestUtils.getBoolean(request, "testConnection", false);
        // 默认开启 OptionRule 校验；纯连接测试或草稿探测可显式关闭。
        // OptionRule validation is on by default; pure connection tests or draft probes may disable
        // it.
        boolean validateOptionRules =
                ProxyRequestUtils.getBoolean(request, "validateOptionRules", !testConnection);
        int timeoutMillis = (int) ProxyRequestUtils.getLong(request, "timeoutMillis", 3500);

        // 如果是仅测试连接，跳过昂贵的表结构全量解析 (skipOfficialDisplayPaths=true)，避免阻塞数十秒
        // If testing connection, skip expensive official table schema resolution to avoid delays
        JobConfigContext context = jobConfigSupportService.parseJobContext(request, testConnection);
        List<String> warnings = filterUserVisibleWarnings(context.getWarnings());
        List<String> infos = new ArrayList<>();
        List<String> errors = new ArrayList<>();
        List<Map<String, Object>> checks = new ArrayList<>();

        if (validateOptionRules && optionRuleValidationService != null) {
            optionRuleValidationService.validate(context, errors, warnings);
        }

        boolean allChecksSucceeded = true;
        if (testConnection) {
            checks = testConnections(context, timeoutMillis);
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
                        infos.add(
                                String.format(
                                        "%s 暂未完成连接测试: %s",
                                        check.get("connectorType"), check.get("message")));
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

    private List<Map<String, Object>> testConnections(JobConfigContext context, int timeoutMillis) {
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

            final ClassLoader effectiveClassLoader = classLoader;
            List<CompletableFuture<Map<String, Object>>> futures = new ArrayList<>();

            for (int i = 0; i < context.getSources().size(); i++) {
                final int index = i;
                futures.add(
                        CompletableFuture.supplyAsync(
                                () ->
                                        testConnectorConnection(
                                                context.getGraph().getNodes().get(index),
                                                context.getSources().get(index),
                                                effectiveClassLoader,
                                                timeoutMillis),
                                PROBE_EXECUTOR));
            }
            int sinkOffset = context.getSources().size() + context.getTransforms().size();
            for (int i = 0; i < context.getSinks().size(); i++) {
                final int index = i;
                futures.add(
                        CompletableFuture.supplyAsync(
                                () ->
                                        testConnectorConnection(
                                                context.getGraph()
                                                        .getNodes()
                                                        .get(sinkOffset + index),
                                                context.getSinks().get(index),
                                                effectiveClassLoader,
                                                timeoutMillis),
                                PROBE_EXECUTOR));
            }

            List<Map<String, Object>> checks = new ArrayList<>();
            for (CompletableFuture<Map<String, Object>> future : futures) {
                try {
                    checks.add(future.get(timeoutMillis + 1500L, TimeUnit.MILLISECONDS));
                } catch (Exception e) {
                    Map<String, Object> fallback = new LinkedHashMap<>();
                    fallback.put("status", "failed");
                    fallback.put("message", "连接测试超时（超过设定时限），请检查网络或服务配置。");
                    checks.add(fallback);
                }
            }
            return checks;
        } finally {
            Thread.currentThread().setContextClassLoader(originalClassLoader);
            PluginClassLoaderUtils.closeQuietly(seatunnelHomeClassLoader);
        }
    }

    private Map<String, Object> testConnectorConnection(
            ProxyNode node, Config config, ClassLoader classLoader, int timeoutMillis) {
        return ConnectionProbeSupport.probeConnector(node, config, classLoader, timeoutMillis);
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
