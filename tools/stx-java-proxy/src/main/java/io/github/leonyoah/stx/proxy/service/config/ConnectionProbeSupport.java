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
import org.apache.seatunnel.api.table.catalog.Catalog;
import org.apache.seatunnel.api.table.factory.FactoryUtil;

import io.github.leonyoah.stx.proxy.model.NodeKind;
import io.github.leonyoah.stx.proxy.model.ProxyNode;

import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.lang.reflect.UndeclaredThrowableException;
import java.net.ConnectException;
import java.net.InetSocketAddress;
import java.net.NoRouteToHostException;
import java.net.Socket;
import java.net.SocketTimeoutException;
import java.net.URI;
import java.net.UnknownHostException;
import java.sql.Connection;
import java.sql.Driver;
import java.sql.DriverManager;
import java.sql.SQLException;
import java.util.Collections;
import java.util.HashMap;
import java.util.IdentityHashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Optional;
import java.util.Properties;
import java.util.Set;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.FutureTask;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * 连接器探测支持类，负责编排各层级探测探针： 1. 本地免检白名单 (Native Local Whitelist) 2. 快速 TCP 预检 3. 官方 DryRun SPI 探测
 * (SupportDryRunValidation) 4. 协议级探针 (Kafka AdminClient / S3 ListObjects / FTP 登录 / SFTP 认证) 5. 官方
 * Catalog SPI 探测 (CatalogFactory & Catalog) 6. JDBC 驱动直连探测 7. 通用 TCP 端口连通性探针 8. 兜底跳过。
 * 整个探测在独立工作线程中执行并受总超时约束，超时后中断工作线程。
 *
 * <p>Connector probing support helper, orchestrating multi-tier probes: native whitelist, fast TCP
 * pre-flight, official DryRun SPI, protocol-level probes (Kafka AdminClient, S3 ListObjects, FTP
 * login, SFTP auth), Catalog SPI, JDBC driver, generic TCP, fallback skip. The whole probe runs on
 * a worker thread under an overall deadline; the worker is interrupted on timeout.
 */
class ConnectionProbeSupport {

    private static final int TCP_TIMEOUT_MS = 1500;

    private static final AtomicInteger WORKER_SEQ = new AtomicInteger();

    /**
     * 探测工作线程池；线程默认上下文类加载器固定为本类加载器，避免把请求级插件类加载器钉在空闲线程上。 Probe worker pool; idle threads keep this
     * class's loader as context loader so request-scoped plugin classloaders are never pinned.
     */
    private static final ExecutorService PROBE_WORKERS =
            Executors.newCachedThreadPool(
                    r -> {
                        Thread t =
                                new Thread(
                                        r,
                                        "stx-connection-probe-worker-"
                                                + WORKER_SEQ.incrementAndGet());
                        t.setDaemon(true);
                        t.setContextClassLoader(ConnectionProbeSupport.class.getClassLoader());
                        return t;
                    });

    private static final Pattern AUTH_MESSAGE =
            Pattern.compile(
                    "(?is).*(access denied|password authentication failed|authentication failed"
                            + "|auth fail|auth cancel|login incorrect|login failed|invalid username"
                            + "|invalid password|invalidaccesskeyid|signaturedoesnotmatch"
                            + "|status code: 403|ora-01017|not authorized|unauthorized"
                            + "|^\\s*530[ -]).*");

    private static final Pattern CONNECTIVITY_MESSAGE =
            Pattern.compile(
                    "(?is).*(communications link failure|connection refused|connect timed out"
                            + "|timed out|timeout|no route to host|unknown host|connection reset"
                            + "|network is unreachable|connection attempt failed).*");

    /** 执行连接器连通性探测（默认 3500ms 超时）。 Execute connector connectivity probe with default timeout. */
    static Map<String, Object> probeConnector(
            ProxyNode node, Config config, ClassLoader classLoader) {
        return probeConnector(node, config, classLoader, 3500);
    }

    /**
     * 执行连接器连通性探测，整体受 timeoutMillis 约束。 Execute connector connectivity probe bounded by an overall
     * timeoutMillis deadline.
     */
    static Map<String, Object> probeConnector(
            ProxyNode node, Config config, ClassLoader classLoader, int timeoutMillis) {
        return startProbe(node, config, classLoader, timeoutMillis).await();
    }

    /**
     * 异步启动探测；调用方通过 {@link ProbeHandle#await()} 取结果，通过 {@link ProbeHandle#completion()}
     * 得知工作线程何时真正结束（用于安全释放类加载器）。 Starts a probe asynchronously. Callers fetch the result with {@link
     * ProbeHandle#await()} and learn when the worker really finished (to release classloaders
     * safely) via {@link ProbeHandle#completion()}.
     */
    static ProbeHandle startProbe(
            ProxyNode node, Config config, ClassLoader classLoader, int timeoutMillis) {
        final int timeout = Math.max(1, timeoutMillis);
        Map<String, Object> base = baseResult(node, config);
        String pluginName = getString(config, "plugin_name");
        if (StringUtils.isBlank(pluginName) && node != null) {
            pluginName = node.getPluginName();
        }
        // Tier 1: 本地免检白名单 (Native Local Whitelist)，无需工作线程。
        if (doesNotRequireConnectionProbe(pluginName)) {
            Map<String, Object> result = new LinkedHashMap<>(base);
            result.put("status", "skipped");
            result.put("severity", "info");
            result.put("probeMethod", "native-local");
            result.put("message", "该连接器不依赖外部服务，无需连接测试。");
            return ProbeHandle.completed(base, result, timeout);
        }
        final String plugin = pluginName;
        final long deadline = System.nanoTime() + TimeUnit.MILLISECONDS.toNanos(timeout);
        FutureTask<Map<String, Object>> task =
                new FutureTask<>(
                        () -> {
                            Thread current = Thread.currentThread();
                            ClassLoader previous = current.getContextClassLoader();
                            if (classLoader != null) {
                                current.setContextClassLoader(classLoader);
                            }
                            try {
                                return runProbeTiers(node, plugin, config, classLoader, deadline);
                            } finally {
                                current.setContextClassLoader(previous);
                            }
                        });
        CompletableFuture<Void> done = new CompletableFuture<>();
        try {
            PROBE_WORKERS.execute(
                    () -> {
                        try {
                            task.run();
                        } finally {
                            done.complete(null);
                        }
                    });
        } catch (RuntimeException e) {
            done.complete(null);
            Map<String, Object> result = new LinkedHashMap<>(base);
            result.put("status", "failed");
            result.put("message", "无法启动连接测试: " + describe(e));
            return ProbeHandle.completed(base, result, timeout);
        }
        return new ProbeHandle(base, task, done, deadline, timeout);
    }

    /** 构造带节点元信息的基础结果。 Builds the base result carrying node metadata. */
    static Map<String, Object> baseResult(ProxyNode node, Config config) {
        String pluginName = config != null ? getString(config, "plugin_name") : null;
        if (StringUtils.isBlank(pluginName) && node != null) {
            pluginName = node.getPluginName();
        }
        Map<String, Object> result = new LinkedHashMap<>();
        if (node != null) {
            String kind = node.getKind().name().toLowerCase(Locale.ROOT);
            result.put("nodeId", node.getNodeId());
            result.put("kind", kind);
            result.put(
                    "connectorType",
                    String.format(
                            "%s[%d]-%s",
                            StringUtils.capitalize(kind), node.getConfigIndex(), pluginName));
        } else {
            result.put("connectorType", StringUtils.defaultString(pluginName, "unknown"));
        }
        result.put(
                "target",
                StringUtils.defaultString(config != null ? resolveDisplayTarget(config) : null));
        return result;
    }

    /** 探测句柄。 Handle of a running probe. */
    static final class ProbeHandle {
        private final Map<String, Object> base;
        private final Future<Map<String, Object>> future;
        private final CompletableFuture<Void> done;
        private final long deadlineNanos;
        private final int timeoutMillis;

        private ProbeHandle(
                Map<String, Object> base,
                Future<Map<String, Object>> future,
                CompletableFuture<Void> done,
                long deadlineNanos,
                int timeoutMillis) {
            this.base = base;
            this.future = future;
            this.done = done;
            this.deadlineNanos = deadlineNanos;
            this.timeoutMillis = timeoutMillis;
        }

        static ProbeHandle completed(
                Map<String, Object> base, Map<String, Object> result, int timeoutMillis) {
            return new ProbeHandle(
                    base,
                    CompletableFuture.completedFuture(result),
                    CompletableFuture.completedFuture(null),
                    System.nanoTime(),
                    timeoutMillis);
        }

        /** 工作线程真正结束（含被中断后退出）时完成。 Completes once the worker has really finished. */
        CompletableFuture<Void> completion() {
            return done;
        }

        /** 带节点元信息的基础结果副本。 Copy of the base result with node metadata. */
        Map<String, Object> baseResult() {
            return new LinkedHashMap<>(base);
        }

        /**
         * 等待结果直到截止时间；超时则中断工作线程并返回失败结果。 Waits until the deadline; on timeout interrupts the worker
         * and returns a failed result.
         */
        Map<String, Object> await() {
            long remaining = deadlineNanos - System.nanoTime();
            try {
                Map<String, Object> result =
                        remaining > 0
                                ? future.get(remaining, TimeUnit.NANOSECONDS)
                                : (future.isDone()
                                        ? future.get()
                                        : future.get(1, TimeUnit.MILLISECONDS));
                Map<String, Object> merged = new LinkedHashMap<>(base);
                if (result != null) {
                    merged.putAll(result);
                }
                return merged;
            } catch (TimeoutException e) {
                future.cancel(true);
                return timeoutResult(base, timeoutMillis);
            } catch (InterruptedException e) {
                future.cancel(true);
                Thread.currentThread().interrupt();
                Map<String, Object> failed = new LinkedHashMap<>(base);
                failed.put("status", "failed");
                failed.put("message", "连接测试被中断。");
                return failed;
            } catch (ExecutionException e) {
                Map<String, Object> failed = new LinkedHashMap<>(base);
                failed.put("status", "failed");
                failed.put("message", describe(e.getCause() != null ? e.getCause() : e));
                return failed;
            } catch (java.util.concurrent.CancellationException e) {
                return timeoutResult(base, timeoutMillis);
            }
        }
    }

    /** 超时失败结果（保留节点元信息）。 Timeout failure result that keeps node metadata. */
    static Map<String, Object> timeoutResult(Map<String, Object> base, int timeoutMillis) {
        Map<String, Object> failed = new LinkedHashMap<>(base);
        failed.put("status", "failed");
        failed.put("probeMethod", "timeout");
        failed.put("message", String.format("连接测试超时（超过 %d ms），请检查网络或服务配置。", timeoutMillis));
        return failed;
    }

    private static int remainingMillis(long deadlineNanos) {
        long remaining = TimeUnit.NANOSECONDS.toMillis(deadlineNanos - System.nanoTime());
        return (int) Math.max(1, Math.min(Integer.MAX_VALUE, remaining));
    }

    private static Map<String, Object> runProbeTiers(
            ProxyNode node,
            String pluginName,
            Config config,
            ClassLoader classLoader,
            long deadlineNanos) {
        ReadonlyConfig readonlyConfig = ReadonlyConfig.fromConfig(config);

        // 快速 TCP 端口预检 (Fast TCP Pre-flight)：端口完全不可达时立即失败。
        // Fast TCP pre-flight: fail immediately when the port is unreachable.
        HostAndPort endpoint = extractEndpoint(config);
        if (endpoint != null) {
            String tcpError =
                    testTcpReachability(
                            endpoint.host,
                            endpoint.port,
                            Math.min(TCP_TIMEOUT_MS, remainingMillis(deadlineNanos)));
            if (tcpError != null) {
                Map<String, Object> result = new LinkedHashMap<>();
                result.put("status", "failed");
                result.put("probeMethod", "tcp-port");
                result.put(
                        "message",
                        String.format(
                                "目标地址网络不可达 (%s:%d): %s", endpoint.host, endpoint.port, tcpError));
                return result;
            }
        }

        // Tier 2: 官方 DryRun SPI 探测 (反射调用以兼容 2.3.x 与 3.0.x 双版本)
        Map<String, Object> dryRunResult =
                tryProbeOfficialDryRun(node, readonlyConfig, classLoader);
        if (dryRunResult != null) {
            return dryRunResult;
        }

        // Tier 3: 协议级探针 (Kafka / S3 / FTP / SFTP)：真正验证认证信息，而不是只看 TCP。
        // Tier 3: protocol-level probes that really verify credentials instead of TCP only.
        Map<String, Object> protocolResult =
                ConnectionProtocolProbes.probe(
                        pluginName, config, classLoader, remainingMillis(deadlineNanos));
        if (protocolResult != null) {
            return protocolResult;
        }

        // Tier 4: 官方 Catalog SPI 探测
        Map<String, Object> catalogResult =
                tryProbeCatalog(pluginName, readonlyConfig, classLoader);
        if (catalogResult != null) {
            return catalogResult;
        }

        // Tier 5: JDBC 驱动直连探测
        if (isJdbcProbeableConnector(pluginName, config)) {
            Map<String, Object> result = new LinkedHashMap<>();
            result.put("probeMethod", "jdbc-driver");
            try {
                probeJdbc(pluginName, config, classLoader, remainingMillis(deadlineNanos));
                result.put("status", "success");
                result.put("message", "Connection succeeded via JDBC driver.");
            } catch (Throwable e) {
                result.put("status", "failed");
                result.put("message", describeFailure(e));
            }
            return result;
        }

        // Tier 6: 通用 TCP 端口连通性探针 (已通过快速预检)。带认证信息时不能报 success。
        // Tier 6: generic TCP probe (pre-flight passed). Never report success when credentials
        // are configured but could not be verified.
        if (endpoint != null) {
            Map<String, Object> res = new LinkedHashMap<>();
            res.put("probeMethod", "tcp-port");
            if (hasCredentials(config)) {
                res.put("status", "skipped");
                res.put("severity", "warning");
                res.put(
                        "message",
                        String.format(
                                "仅验证了网络可达 (%s:%d)，该连接器暂无协议级探针，认证信息未验证。",
                                endpoint.host, endpoint.port));
            } else {
                res.put("status", "success");
                res.put(
                        "message",
                        String.format(
                                "TCP network reachability verified (%s:%d).",
                                endpoint.host, endpoint.port));
            }
            return res;
        }

        // Tier 7: 平台未覆盖专有探针，安全跳过作为 Info
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("status", "skipped");
        result.put("severity", "info");
        result.put("probeMethod", "unsupported-skip");
        result.put("message", "该连接器暂未配置专用协议探针（已跳过，不阻断任务校验）。");
        return result;
    }

    /**
     * 判断连接器是否天然不需要外部连接探测。 Detects connectors that naturally do not need external connection
     * probing.
     */
    static boolean doesNotRequireConnectionProbe(String pluginName) {
        return "FakeSource".equalsIgnoreCase(pluginName)
                || "Console".equalsIgnoreCase(pluginName)
                || "Assert".equalsIgnoreCase(pluginName)
                || "LocalFile".equalsIgnoreCase(pluginName);
    }

    /**
     * 尝试通过 SeaTunnel 3.0+ 的 SupportSourceDryRunValidation 探测。 Try probing via SeaTunnel 3.0+
     * SupportSourceDryRunValidation.
     */
    private static Map<String, Object> tryProbeOfficialDryRun(
            ProxyNode node, ReadonlyConfig readonlyConfig, ClassLoader classLoader) {
        if (node == null || node.getKind() != NodeKind.SOURCE) {
            return null;
        }
        Object factory;
        Class<?> dryRunSourceClass;
        Class<?> ctxClass;
        try {
            try {
                dryRunSourceClass =
                        Class.forName(
                                "org.apache.seatunnel.api.table.factory.SupportSourceDryRunValidation",
                                false,
                                classLoader);
            } catch (ClassNotFoundException ignored) {
                return null;
            }
            Class<?> sourceFactoryClass =
                    Class.forName(
                            "org.apache.seatunnel.api.table.factory.TableSourceFactory",
                            false,
                            classLoader);
            Method discoverMethod =
                    FactoryUtil.class.getMethod(
                            "discoverOptionalFactory",
                            ClassLoader.class,
                            Class.class,
                            String.class);
            Optional<?> optionalFactory =
                    (Optional<?>)
                            discoverMethod.invoke(
                                    null, classLoader, sourceFactoryClass, node.getPluginName());
            if (!optionalFactory.isPresent()
                    || !dryRunSourceClass.isInstance(optionalFactory.get())) {
                return null;
            }
            factory = optionalFactory.get();
            ctxClass =
                    Class.forName(
                            "org.apache.seatunnel.api.table.factory.TableSourceFactoryContext",
                            false,
                            classLoader);
        } catch (Throwable t) {
            return null;
        }
        try {
            Object ctx =
                    ctxClass.getConstructor(ReadonlyConfig.class, ClassLoader.class)
                            .newInstance(readonlyConfig, classLoader);
            Method inferMethod = dryRunSourceClass.getMethod("inferSchemaForDryRun", ctxClass);
            Object schemas = inferMethod.invoke(factory, ctx);
            Method validateMethod =
                    dryRunSourceClass.getMethod(
                            "validateConnectionForDryRun", ctxClass, List.class);
            validateMethod.invoke(factory, ctx, schemas);

            Map<String, Object> res = new LinkedHashMap<>();
            res.put("status", "success");
            res.put("probeMethod", "dry-run-spi");
            res.put(
                    "message",
                    "Connection and metadata probed successfully via official DryRun SPI.");
            return res;
        } catch (Throwable t) {
            // 沿 cause 链识别认证/网络错误；配置不兼容类错误降级到后续层级。
            // Classify along the cause chain; config-incompatibility errors fall through.
            if (findAuthCause(t) != null || findConnectivityCause(t) != null) {
                return failure("dry-run-spi", t);
            }
            return null;
        }
    }

    /**
     * 尝试通过 SeaTunnel CatalogFactory 动态发现并打开 Catalog 进行连接探测。 Try discovering and opening Catalog via
     * SeaTunnel CatalogFactory for connectivity probe.
     */
    private static Map<String, Object> tryProbeCatalog(
            String pluginName, ReadonlyConfig readonlyConfig, ClassLoader classLoader) {
        if (StringUtils.isBlank(pluginName)) {
            return null;
        }
        String catalogIdentifier = pluginName;
        if (pluginName.endsWith("-CDC")) {
            // 对齐 SeaTunnel 官方 CatalogTableUtil 规范：剥离 -CDC 路由至底层数据库 Catalog
            catalogIdentifier = pluginName.substring(0, pluginName.length() - 4);
        }
        Optional<Catalog> optionalCatalog;
        try {
            optionalCatalog =
                    FactoryUtil.createOptionalCatalog(
                            "stx_probe_" + catalogIdentifier,
                            readonlyConfig,
                            classLoader,
                            catalogIdentifier);
        } catch (Throwable t) {
            // 创建阶段失败多为配置形态不匹配（如 CDC 未写 base-url），降级到后续层级；认证/网络错误除外。
            // Creation failures are mostly config-shape mismatches (fall through), except
            // auth/connectivity errors.
            if (findAuthCause(t) != null || findConnectivityCause(t) != null) {
                return failure("catalog-spi", t);
            }
            return null;
        }
        if (!optionalCatalog.isPresent()) {
            return null;
        }
        Catalog catalog = optionalCatalog.get();
        try {
            catalog.open();
            Map<String, Object> res = new LinkedHashMap<>();
            res.put("status", "success");
            res.put("probeMethod", "catalog-spi");
            res.put("message", "Connected successfully via " + catalogIdentifier + " Catalog.");
            return res;
        } catch (Throwable t) {
            // open() 真正发起了连接：除类缺失等环境问题外，一律判失败，绝不回落到 TCP 成功。
            // open() really connected: anything but missing-class problems is a failure, never a
            // fallback to TCP success.
            if (findAuthCause(t) == null
                    && findConnectivityCause(t) == null
                    && isEnvironmentProblem(t)) {
                return null;
            }
            return failure("catalog-spi", t);
        } finally {
            try {
                catalog.close();
            } catch (Throwable ignored) {
                // no-op
            }
        }
    }

    private static Map<String, Object> failure(String probeMethod, Throwable t) {
        Map<String, Object> failRes = new LinkedHashMap<>();
        failRes.put("status", "failed");
        failRes.put("probeMethod", probeMethod);
        failRes.put("message", describeFailure(t));
        return failRes;
    }

    /** 生成失败描述；认证失败显式标注。 Builds a failure description; auth failures are labelled. */
    static String describeFailure(Throwable t) {
        Throwable auth = findAuthCause(t);
        if (auth != null) {
            String msg = firstLine(auth.getMessage());
            String state = null;
            if (auth instanceof SQLException) {
                state = ((SQLException) auth).getSQLState();
            }
            return "认证失败 (authentication failed): "
                    + msg
                    + (StringUtils.isNotBlank(state) && !msg.contains(state)
                            ? " [SQLState " + state + "]"
                            : "");
        }
        return describe(t);
    }

    /** 外层消息 + 根因消息。 Outer message plus root-cause message. */
    static String describe(Throwable t) {
        Throwable outer = unwrapReflection(t);
        Throwable root = outer;
        for (Throwable c : causeChain(outer)) {
            if (StringUtils.isNotBlank(c.getMessage())) {
                root = c;
            }
        }
        String outerMsg =
                StringUtils.isNotBlank(outer.getMessage())
                        ? firstLine(outer.getMessage())
                        : outer.getClass().getSimpleName();
        if (root != outer && StringUtils.isNotBlank(root.getMessage())) {
            String rootMsg = firstLine(root.getMessage());
            if (!outerMsg.contains(rootMsg)) {
                return outerMsg + " (cause: " + rootMsg + ")";
            }
        }
        return outerMsg;
    }

    private static Throwable unwrapReflection(Throwable t) {
        Throwable current = t;
        for (int i = 0; i < 10 && current != null; i++) {
            Throwable next = null;
            if (current instanceof InvocationTargetException) {
                next = ((InvocationTargetException) current).getTargetException();
            } else if (current instanceof UndeclaredThrowableException) {
                next = ((UndeclaredThrowableException) current).getUndeclaredThrowable();
            } else if (current instanceof ExecutionException) {
                next = current.getCause();
            }
            if (next == null) {
                break;
            }
            current = next;
        }
        return current == null ? t : current;
    }

    /** 展开 cause 链（含反射包装与 SQLException.getNextException）。 Flattens the cause chain. */
    static List<Throwable> causeChain(Throwable t) {
        List<Throwable> chain = new java.util.ArrayList<>();
        Set<Throwable> seen = Collections.newSetFromMap(new IdentityHashMap<>());
        java.util.Deque<Throwable> queue = new java.util.ArrayDeque<>();
        if (t != null) {
            queue.add(t);
        }
        while (!queue.isEmpty() && chain.size() < 64) {
            Throwable current = queue.poll();
            if (current == null || !seen.add(current)) {
                continue;
            }
            chain.add(current);
            if (current instanceof InvocationTargetException) {
                addIfPresent(queue, ((InvocationTargetException) current).getTargetException());
            }
            if (current instanceof UndeclaredThrowableException) {
                addIfPresent(
                        queue, ((UndeclaredThrowableException) current).getUndeclaredThrowable());
            }
            if (current instanceof SQLException) {
                addIfPresent(queue, ((SQLException) current).getNextException());
            }
            addIfPresent(queue, current.getCause());
            for (Throwable suppressed : current.getSuppressed()) {
                addIfPresent(queue, suppressed);
            }
        }
        return chain;
    }

    private static void addIfPresent(java.util.Deque<Throwable> queue, Throwable t) {
        if (t != null) {
            queue.add(t);
        }
    }

    /**
     * 沿 cause 链查找认证失败：SQLState 28xxx、Sasl/Authentication 异常、S3 403、FTP 530、JSch Auth fail 等。 Finds
     * an authentication failure along the cause chain.
     */
    static Throwable findAuthCause(Throwable t) {
        for (Throwable c : causeChain(t)) {
            if (isAuthFailure(c)) {
                return c;
            }
        }
        return null;
    }

    private static boolean isAuthFailure(Throwable c) {
        if (c instanceof SQLException) {
            SQLException sql = (SQLException) c;
            String state = sql.getSQLState();
            if (state != null && state.startsWith("28")) {
                return true;
            }
            // MySQL 1045 / SQLServer 18456 / Oracle 1017
            int code = sql.getErrorCode();
            if (code == 1045 || code == 18456 || code == 1017) {
                return true;
            }
        }
        for (Class<?> cls = c.getClass();
                cls != null && cls != Object.class;
                cls = cls.getSuperclass()) {
            String name = cls.getName();
            if (name.endsWith("AuthenticationException")
                    || name.endsWith("AuthorizationException")
                    || name.endsWith("LoginException")
                    || name.endsWith("AccessDeniedException")) {
                return true;
            }
        }
        String msg = c.getMessage();
        return msg != null && AUTH_MESSAGE.matcher(msg).matches();
    }

    /** 沿 cause 链查找网络类错误。 Finds a network/connectivity failure along the cause chain. */
    static Throwable findConnectivityCause(Throwable t) {
        for (Throwable c : causeChain(t)) {
            if (c instanceof ConnectException
                    || c instanceof SocketTimeoutException
                    || c instanceof UnknownHostException
                    || c instanceof NoRouteToHostException
                    || c instanceof TimeoutException
                    || c instanceof InterruptedException
                    || c.getClass().getName().endsWith("CommunicationsException")) {
                return c;
            }
            if (c instanceof SQLException) {
                String state = ((SQLException) c).getSQLState();
                if (state != null && state.startsWith("08")) {
                    return c;
                }
            }
            String msg = c.getMessage();
            if (msg != null && CONNECTIVITY_MESSAGE.matcher(msg).matches()) {
                return c;
            }
        }
        return null;
    }

    /** 类缺失/不支持等运行环境问题（可降级到后续层级）。 Missing-class style environment problems. */
    private static boolean isEnvironmentProblem(Throwable t) {
        for (Throwable c : causeChain(t)) {
            if (c instanceof ClassNotFoundException
                    || c instanceof LinkageError
                    || c instanceof UnsupportedOperationException) {
                return true;
            }
        }
        return false;
    }

    /** 判断连接器是否可以通过 JDBC 驱动进行连接探测。 Checks whether the connector can be probed via JDBC driver. */
    static boolean isJdbcProbeableConnector(String pluginName, Config config) {
        if ("Jdbc".equalsIgnoreCase(pluginName)
                || "MySQL-CDC".equalsIgnoreCase(pluginName)
                || "Postgres-CDC".equalsIgnoreCase(pluginName)
                || "SqlServer-CDC".equalsIgnoreCase(pluginName)
                || "Oracle-CDC".equalsIgnoreCase(pluginName)) {
            return true;
        }
        String url = getString(config, "url");
        return StringUtils.isNotBlank(url) && url.toLowerCase(Locale.ROOT).startsWith("jdbc:");
    }

    /**
     * 解析配置中的数据库名称（兼容 database-name 与 database-names 列表）。 Resolves the database name from config
     * (supports database-name and database-names list).
     */
    private static String resolveDatabaseName(Config config, String defaultDb) {
        String db = getString(config, "database-name");
        if (StringUtils.isNotBlank(db)) {
            return db;
        }
        if (config.hasPath("database-names")) {
            try {
                List<String> list = config.getStringList("database-names");
                if (list != null && !list.isEmpty() && StringUtils.isNotBlank(list.get(0))) {
                    return list.get(0);
                }
            } catch (Exception ignored) {
                // no-op
            }
        }
        return defaultDb;
    }

    /** 解析 JDBC URL：url 优先，其次 CDC 的 base-url，最后由 hostname/port 拼装。 Resolves the JDBC URL. */
    static String resolveJdbcUrl(String pluginName, Config config) {
        String url = getString(config, "url");
        if (StringUtils.isBlank(url)) {
            url = getString(config, "base-url");
        }
        if (StringUtils.isNotBlank(url)) {
            return url;
        }
        String hostname = getString(config, "hostname");
        if (StringUtils.isBlank(hostname)) {
            hostname = getString(config, "host");
        }
        if (StringUtils.isBlank(hostname)) {
            return null;
        }
        String port = getString(config, "port");
        if ("MySQL-CDC".equalsIgnoreCase(pluginName)) {
            return String.format(
                    "jdbc:mysql://%s:%s/", hostname, StringUtils.defaultIfBlank(port, "3306"));
        } else if ("Postgres-CDC".equalsIgnoreCase(pluginName)) {
            String db = resolveDatabaseName(config, "postgres");
            return String.format(
                    "jdbc:postgresql://%s:%s/%s",
                    hostname, StringUtils.defaultIfBlank(port, "5432"), db);
        } else if ("SqlServer-CDC".equalsIgnoreCase(pluginName)) {
            String db = resolveDatabaseName(config, "master");
            return String.format(
                    "jdbc:sqlserver://%s:%s;databaseName=%s",
                    hostname, StringUtils.defaultIfBlank(port, "1433"), db);
        } else if ("Oracle-CDC".equalsIgnoreCase(pluginName)) {
            String db = resolveDatabaseName(config, "XE");
            return String.format(
                    "jdbc:oracle:thin:@//%s:%s/%s",
                    hostname, StringUtils.defaultIfBlank(port, "1521"), db);
        }
        return null;
    }

    /** 执行 JDBC 驱动物理建连探测。 Probe JDBC connection via driver directly. */
    static void probeJdbc(String pluginName, Config config, ClassLoader classLoader)
            throws Exception {
        probeJdbc(pluginName, config, classLoader, 3500);
    }

    static void probeJdbc(
            String pluginName, Config config, ClassLoader classLoader, int timeoutMillis)
            throws Exception {
        String url = resolveJdbcUrl(pluginName, config);
        if (StringUtils.isBlank(url)) {
            throw new IllegalArgumentException(
                    "Missing required connection option: url, base-url or hostname.");
        }

        String username = getString(config, "username");
        if (StringUtils.isBlank(username)) {
            username = getString(config, "user");
        }
        String password = getString(config, "password");
        String driverClassName = getString(config, "driver");
        if (StringUtils.isBlank(driverClassName)) {
            driverClassName = resolveDefaultJdbcDriver(pluginName, url);
        }
        if (StringUtils.isBlank(driverClassName)) {
            throw new IllegalArgumentException("Missing jdbc driver option.");
        }

        Class<?> driverClass = Class.forName(driverClassName, true, classLoader);
        Driver driver = (Driver) driverClass.getDeclaredConstructor().newInstance();
        Properties properties = new Properties();
        if (StringUtils.isNotBlank(username)) {
            properties.setProperty("user", username);
        }
        if (password != null) {
            properties.setProperty("password", password);
        }
        applyDriverTimeouts(url, properties, timeoutMillis);
        DriverManager.setLoginTimeout(Math.max(1, timeoutMillis / 1000));
        try (Connection connection = driver.connect(url, properties)) {
            if (connection == null) {
                throw new IllegalStateException(
                        "JDBC driver " + driverClassName + " does not accept url " + url);
            }
        }
    }

    /**
     * 按驱动类型设置连接/读超时，避免握手阶段无限阻塞（仅在 URL 未显式配置时设置）。 Sets driver-specific connect/read timeouts so a
     * silent server cannot block forever (only when not set in the URL).
     */
    private static void applyDriverTimeouts(String url, Properties properties, int timeoutMillis) {
        String lower = url.toLowerCase(Locale.ROOT);
        int ms = Math.max(500, timeoutMillis);
        int seconds = Math.max(1, (ms + 999) / 1000);
        if (lower.startsWith("jdbc:mysql") || lower.startsWith("jdbc:mariadb")) {
            putIfAbsentInUrl(lower, properties, "connectTimeout", String.valueOf(ms));
            putIfAbsentInUrl(lower, properties, "socketTimeout", String.valueOf(ms));
        } else if (lower.startsWith("jdbc:postgresql")) {
            putIfAbsentInUrl(lower, properties, "connectTimeout", String.valueOf(seconds));
            putIfAbsentInUrl(lower, properties, "socketTimeout", String.valueOf(seconds));
            putIfAbsentInUrl(lower, properties, "loginTimeout", String.valueOf(seconds));
        } else if (lower.startsWith("jdbc:sqlserver")) {
            putIfAbsentInUrl(lower, properties, "loginTimeout", String.valueOf(seconds));
            putIfAbsentInUrl(lower, properties, "socketTimeout", String.valueOf(ms));
        } else if (lower.startsWith("jdbc:oracle")) {
            putIfAbsentInUrl(lower, properties, "oracle.net.CONNECT_TIMEOUT", String.valueOf(ms));
            putIfAbsentInUrl(lower, properties, "oracle.jdbc.ReadTimeout", String.valueOf(ms));
        }
    }

    private static void putIfAbsentInUrl(
            String lowerUrl, Properties properties, String key, String value) {
        if (!lowerUrl.contains(key.toLowerCase(Locale.ROOT) + "=")) {
            properties.setProperty(key, value);
        }
    }

    /** 测试指定主机端口的 TCP 握手连通性。 Tests TCP handshake reachability for given host and port. */
    static String testTcpReachability(String host, int port, int timeoutMs) {
        try (Socket socket = new Socket()) {
            socket.connect(new InetSocketAddress(host, port), Math.max(500, timeoutMs));
            return null;
        } catch (Exception e) {
            return firstLine(e.getMessage());
        }
    }

    /** 通用 TCP Socket 端口连通性探针（三次握手探活）。 Generic TCP Socket Port Probe (3-way handshake). */
    static Map<String, Object> tryProbeGenericTcp(Config config) {
        HostAndPort endpoint = extractEndpoint(config);
        if (endpoint == null) {
            return null;
        }
        Map<String, Object> res = new LinkedHashMap<>();
        String error = testTcpReachability(endpoint.host, endpoint.port, TCP_TIMEOUT_MS);
        res.put("probeMethod", "tcp-port");
        if (error == null) {
            res.put("status", "success");
            res.put(
                    "message",
                    String.format(
                            "TCP network reachability verified (%s:%d).",
                            endpoint.host, endpoint.port));
        } else {
            res.put("status", "failed");
            res.put(
                    "message",
                    String.format(
                            "TCP connection failed (%s:%d): %s",
                            endpoint.host, endpoint.port, error));
        }
        return res;
    }

    static final class HostAndPort {
        final String host;
        final int port;

        HostAndPort(String host, int port) {
            this.host = host;
            this.port = port;
        }
    }

    /** 各连接器协议的默认端口（host 未带端口时使用）。 Default port per connector protocol. */
    private static final Map<String, Integer> DEFAULT_PORTS;

    static {
        Map<String, Integer> ports = new HashMap<>();
        ports.put("mysql-cdc", 3306);
        ports.put("tidb-cdc", 4000);
        ports.put("postgres-cdc", 5432);
        ports.put("sqlserver-cdc", 1433);
        ports.put("oracle-cdc", 1521);
        ports.put("ftpfile", 21);
        ports.put("sftpfile", 22);
        ports.put("redis", 6379);
        ports.put("mongodb", 27017);
        ports.put("mongodb-cdc", 27017);
        ports.put("cassandra", 9042);
        ports.put("clickhouse", 8123);
        ports.put("kafka", 9092);
        DEFAULT_PORTS = Collections.unmodifiableMap(ports);
    }

    static Integer defaultPortFor(String pluginName) {
        return pluginName == null ? null : DEFAULT_PORTS.get(pluginName.toLowerCase(Locale.ROOT));
    }

    private static int resolveDefaultJdbcPort(String url) {
        String lower = url.toLowerCase(Locale.ROOT);
        if (lower.startsWith("jdbc:postgresql")) {
            return 5432;
        }
        if (lower.startsWith("jdbc:oracle")) {
            return 1521;
        }
        if (lower.startsWith("jdbc:sqlserver")) {
            return 1433;
        }
        if (lower.startsWith("jdbc:clickhouse")) {
            return 8123;
        }
        if (lower.startsWith("jdbc:db2")) {
            return 50000;
        }
        if (lower.startsWith("jdbc:dm")) {
            return 5236;
        }
        return 3306;
    }

    private static final Pattern JDBC_AUTHORITY =
            Pattern.compile("^jdbc:[^/]*//([^/:?#;,\\\\()]+)(?::([0-9]+))?");
    private static final Pattern ORACLE_THIN_AT =
            Pattern.compile("(?i)^jdbc:oracle:thin:[^@]*@(?!//)([^:/@()]+):([0-9]+)");
    private static final Pattern TNS_HOST_PORT =
            Pattern.compile(
                    "(?i)\\(HOST\\s*=\\s*([^)\\s]+)\\s*\\).*?\\(PORT\\s*=\\s*([0-9]+)\\s*\\)");

    /** 从 JDBC URL 提取主机与端口。 Extracts host and port from a JDBC URL. */
    static HostAndPort parseJdbcEndpoint(String url) {
        if (StringUtils.isBlank(url)) {
            return null;
        }
        String trimmed = url.trim();
        Matcher oracle = ORACLE_THIN_AT.matcher(trimmed);
        if (oracle.find()) {
            return new HostAndPort(oracle.group(1), Integer.parseInt(oracle.group(2)));
        }
        Matcher tns = TNS_HOST_PORT.matcher(trimmed);
        if (tns.find()) {
            return new HostAndPort(tns.group(1), Integer.parseInt(tns.group(2)));
        }
        Matcher matcher = JDBC_AUTHORITY.matcher(trimmed);
        if (matcher.find()) {
            String host = matcher.group(1);
            String portStr = matcher.group(2);
            int port =
                    StringUtils.isNotBlank(portStr)
                            ? Integer.parseInt(portStr)
                            : resolveDefaultJdbcPort(trimmed);
            return new HostAndPort(host, port);
        }
        return null;
    }

    private static HostAndPort parseHttpEndpoint(String url) {
        try {
            URI uri = URI.create(url.trim());
            if (StringUtils.isBlank(uri.getHost())) {
                return null;
            }
            int port =
                    uri.getPort() != -1
                            ? uri.getPort()
                            : ("https".equalsIgnoreCase(uri.getScheme()) ? 443 : 80);
            return new HostAndPort(uri.getHost(), port);
        } catch (Exception e) {
            return null;
        }
    }

    static HostAndPort extractEndpoint(Config config) {
        String pluginName = getString(config, "plugin_name");
        for (String key : new String[] {"url", "base-url"}) {
            String url = getString(config, key);
            if (StringUtils.isBlank(url)) {
                continue;
            }
            String lower = url.trim().toLowerCase(Locale.ROOT);
            if (lower.startsWith("jdbc:")) {
                HostAndPort hp = parseJdbcEndpoint(url);
                if (hp != null) {
                    return hp;
                }
            } else if (lower.startsWith("http://") || lower.startsWith("https://")) {
                HostAndPort hp = parseHttpEndpoint(url);
                if (hp != null) {
                    return hp;
                }
            }
        }
        String host = getString(config, "hostname");
        if (StringUtils.isBlank(host)) {
            host = getString(config, "host");
        }
        if (StringUtils.isNotBlank(host)) {
            host = host.trim();
            Integer defaultPort = defaultPortFor(pluginName);
            String portStr = getString(config, "port");
            if (StringUtils.isNotBlank(portStr)) {
                try {
                    return new HostAndPort(stripPort(host), Integer.parseInt(portStr.trim()));
                } catch (NumberFormatException ignored) {
                    // fall through to host-embedded / default port
                }
            }
            if (host.indexOf(':') > 0 && host.indexOf(':') == host.lastIndexOf(':')) {
                HostAndPort hp =
                        parseHostAndPortString(
                                host.split(",")[0], defaultPort == null ? -1 : defaultPort);
                if (hp != null && hp.port > 0) {
                    return hp;
                }
            }
            // 未知协议且未写端口时不猜测端口（旧逻辑会错误地回落到 80）。
            // Do not guess a port for unknown protocols (the old code wrongly fell back to 80).
            return defaultPort == null ? null : new HostAndPort(host, defaultPort);
        }
        String bootstrap = getString(config, "bootstrap.servers");
        if (StringUtils.isNotBlank(bootstrap)) {
            HostAndPort hp = parseHostAndPortString(bootstrap.split(",")[0].trim(), 9092);
            if (hp != null) {
                return hp;
            }
        }
        // Doris: 提取 fenodes (如 "127.0.0.1:8030")
        String fenodes = getString(config, "fenodes");
        if (StringUtils.isNotBlank(fenodes)) {
            HostAndPort hp = parseHostAndPortString(fenodes.split(",")[0].trim(), 8030);
            if (hp != null) {
                return hp;
            }
        }
        // Hive: 提取 metastore_uri / hive.metastore.uris (如 "thrift://127.0.0.1:9083")
        String metastoreUri = getString(config, "metastore_uri");
        if (StringUtils.isBlank(metastoreUri)) {
            metastoreUri = getString(config, "hive.metastore.uris");
        }
        if (StringUtils.isNotBlank(metastoreUri)) {
            String first = metastoreUri.split(",")[0].trim();
            if (first.startsWith("thrift://")) {
                first = first.substring("thrift://".length());
            }
            HostAndPort hp = parseHostAndPortString(first, 9083);
            if (hp != null) {
                return hp;
            }
        }
        // S3 / MinIO / OSS: 提取 fs.s3a.endpoint / s3.endpoint / endpoint
        String s3Endpoint = resolveS3Endpoint(config);
        if (StringUtils.isNotBlank(s3Endpoint)) {
            String ep = s3Endpoint.trim();
            String lower = ep.toLowerCase(Locale.ROOT);
            if (lower.startsWith("http://") || lower.startsWith("https://")) {
                HostAndPort hp = parseHttpEndpoint(ep);
                if (hp != null) {
                    return hp;
                }
            } else {
                HostAndPort hp = parseHostAndPortString(ep, 443);
                if (hp != null) {
                    return hp;
                }
            }
        }
        return null;
    }

    static String resolveS3Endpoint(Config config) {
        String s3Endpoint = getString(config, "fs.s3a.endpoint");
        if (StringUtils.isBlank(s3Endpoint)) {
            s3Endpoint = getString(config, "s3.endpoint");
        }
        if (StringUtils.isBlank(s3Endpoint)) {
            s3Endpoint = getString(config, "endpoint");
        }
        return s3Endpoint;
    }

    private static String stripPort(String host) {
        int idx = host.indexOf(':');
        return idx > 0 && idx == host.lastIndexOf(':') ? host.substring(0, idx) : host;
    }

    /**
     * 解析形如 host:port 的字符串，缺省端口时使用默认值。 Parses host:port string, falling back to default port when
     * omitted.
     */
    static HostAndPort parseHostAndPortString(String hostPortStr, int defaultPort) {
        if (StringUtils.isBlank(hostPortStr)) {
            return null;
        }
        String cleaned = hostPortStr.trim();
        int scheme = cleaned.indexOf("://");
        if (scheme > 0) {
            cleaned = cleaned.substring(scheme + 3);
        }
        if (cleaned.contains("/")) {
            cleaned = cleaned.substring(0, cleaned.indexOf('/'));
        }
        String[] parts = cleaned.split(":");
        if (parts.length >= 2) {
            try {
                return new HostAndPort(parts[0].trim(), Integer.parseInt(parts[1].trim()));
            } catch (NumberFormatException ignored) {
                // no-op
            }
        } else if (parts.length == 1 && StringUtils.isNotBlank(parts[0])) {
            return new HostAndPort(parts[0].trim(), defaultPort);
        }
        return null;
    }

    private static String resolveDisplayTarget(Config config) {
        String target = getString(config, "url");
        if (StringUtils.isBlank(target)) {
            target = getString(config, "base-url");
        }
        if (StringUtils.isBlank(target)) {
            String host = getString(config, "hostname");
            if (StringUtils.isBlank(host)) {
                host = getString(config, "host");
            }
            if (StringUtils.isNotBlank(host)) {
                String port = getString(config, "port");
                target = StringUtils.isNotBlank(port) ? host + ":" + port : host;
            }
        }
        if (StringUtils.isBlank(target)) {
            target = getString(config, "bootstrap.servers");
        }
        if (StringUtils.isBlank(target)) {
            target = getString(config, "fenodes");
        }
        if (StringUtils.isBlank(target)) {
            target = getString(config, "metastore_uri");
            if (StringUtils.isBlank(target)) {
                target = getString(config, "hive.metastore.uris");
            }
        }
        if (StringUtils.isBlank(target)) {
            target = resolveS3Endpoint(config);
        }
        return target;
    }

    /** 配置中是否包含需验证的认证信息。 Whether the config carries credentials that need verifying. */
    static boolean hasCredentials(Config config) {
        for (String key :
                new String[] {
                    "password",
                    "passwd",
                    "secret_key",
                    "access_key",
                    "fs.s3a.secret.key",
                    "token",
                    "kafka.config.sasl.jaas.config",
                    "kerberos_principal",
                    "kerberos_keytab_path"
                }) {
            if (StringUtils.isNotBlank(getString(config, key))) {
                return true;
            }
        }
        return ConnectionProtocolProbes.kafkaClientProperties(config)
                .containsKey("sasl.jaas.config");
    }

    private static String resolveDefaultJdbcDriver(String pluginName, String url) {
        String lower = url == null ? "" : url.toLowerCase(Locale.ROOT);
        if ("MySQL-CDC".equalsIgnoreCase(pluginName) || lower.startsWith("jdbc:mysql")) {
            return "com.mysql.cj.jdbc.Driver";
        }
        if ("Postgres-CDC".equalsIgnoreCase(pluginName) || lower.startsWith("jdbc:postgresql")) {
            return "org.postgresql.Driver";
        }
        if ("SqlServer-CDC".equalsIgnoreCase(pluginName) || lower.startsWith("jdbc:sqlserver")) {
            return "com.microsoft.sqlserver.jdbc.SQLServerDriver";
        }
        if ("Oracle-CDC".equalsIgnoreCase(pluginName) || lower.startsWith("jdbc:oracle")) {
            return "oracle.jdbc.OracleDriver";
        }
        return null;
    }

    static String getString(Config config, String path) {
        if (config == null) {
            return null;
        }
        try {
            if (config.hasPath(path)) {
                return config.getString(path);
            }
            // 兼容以字面量带点 key 书写的配置（如 "bootstrap.servers" = ...）。
            // Also support literal dotted keys such as "bootstrap.servers" = ...
            if (path.indexOf('.') > 0) {
                String quoted = "\"" + path + "\"";
                if (config.hasPath(quoted)) {
                    return config.getString(quoted);
                }
            }
        } catch (Exception ignored) {
            // Non-string values (objects/lists) are not usable here.
        }
        return null;
    }

    static String firstLine(String message) {
        if (StringUtils.isBlank(message)) {
            return "unknown error";
        }
        int newline = message.indexOf('\n');
        return newline >= 0 ? message.substring(0, newline).trim() : message.trim();
    }
}
