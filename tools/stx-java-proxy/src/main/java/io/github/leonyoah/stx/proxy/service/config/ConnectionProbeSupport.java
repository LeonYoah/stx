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

import java.lang.reflect.Method;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.net.URI;
import java.sql.Connection;
import java.sql.Driver;
import java.sql.DriverManager;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.Properties;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

import static org.apache.seatunnel.api.options.ConnectorCommonOptions.PLUGIN_NAME;

/**
 * 连接器探测支持类，负责编排各层级探测探针： 1. 本地免检白名单 (Native Local Whitelist) 2. 官方 DryRun SPI 探测
 * (SupportDryRunValidation) 3. 官方 Catalog SPI 探测 (CatalogFactory & Catalog) 4. CDC 官方 Catalog 映射与
 * JDBC 驱动直连探测 5. 通用 TCP 端口连通性探针 (Generic TCP Socket Probe) 6. 兜底跳过 (Fallback Skip as Info)
 *
 * <p>Connector probing support helper, orchestrating multi-tier probes: 1. Native Local Whitelist
 * 2. Official DryRun SPI Probe (SupportDryRunValidation) 3. Official Catalog SPI Probe
 * (CatalogFactory & Catalog) 4. CDC to Catalog Mapping & JDBC Driver Probe 5. Generic TCP Socket
 * Port Probe 6. Fallback Skip as Info
 */
class ConnectionProbeSupport {

    private static final int TCP_TIMEOUT_MS = 1500;

    /** 执行连接器连通性探测（默认 3500ms 超时）。 Execute connector connectivity probe with default timeout. */
    static Map<String, Object> probeConnector(
            ProxyNode node, Config config, ClassLoader classLoader) {
        return probeConnector(node, config, classLoader, 3500);
    }

    /** 执行连接器连通性探测。 Execute connector connectivity probe with specified timeout. */
    static Map<String, Object> probeConnector(
            ProxyNode node, Config config, ClassLoader classLoader, int timeoutMillis) {
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

        String target = resolveDisplayTarget(config);
        result.put("target", StringUtils.defaultString(target));

        // Tier 1: 本地免检白名单 (Native Local Whitelist)
        if (doesNotRequireConnectionProbe(pluginName)) {
            result.put("status", "skipped");
            result.put("severity", "info");
            result.put("probeMethod", "native-local");
            result.put("message", "该连接器不依赖外部服务，无需连接测试。");
            return result;
        }

        // 快速 TCP 端口预检 (Fast TCP Pre-flight):
        // 若配置包含可解析的 Host/Port 或 URL，先执行快速 TCP 握手 (超时 1500ms)。
        // 若端口完全不可达，立即快速失败，避免后续 Catalog/JDBC 阻塞 30 秒操作系统 SYN 超时。
        HostAndPort endpoint = extractEndpoint(config);
        if (endpoint != null) {
            String tcpError =
                    testTcpReachability(
                            endpoint.host, endpoint.port, Math.min(TCP_TIMEOUT_MS, timeoutMillis));
            if (tcpError != null) {
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
        // Tier 2: Official DryRun SPI Probe (reflectively invoked for dual 2.3.x / 3.0.x
        // compatibility)
        Map<String, Object> dryRunResult =
                tryProbeOfficialDryRun(node, readonlyConfig, classLoader);
        if (dryRunResult != null) {
            result.putAll(dryRunResult);
            return result;
        }

        // Tier 3: 官方 Catalog SPI 探测 (通过 FactoryUtil 动态创建并打开 Catalog)
        // Tier 3: Official Catalog SPI Probe (dynamically created and opened via FactoryUtil)
        Map<String, Object> catalogResult =
                tryProbeCatalog(pluginName, readonlyConfig, classLoader);
        if (catalogResult != null) {
            result.putAll(catalogResult);
            return result;
        }

        // Tier 4: JDBC 驱动直连探测 (适用于标准 JDBC 以及包含 JDBC URL 的 CDC 连接器)
        // Tier 4: Direct JDBC driver probe (for standard JDBC and CDC connectors with JDBC URLs)
        if (isJdbcProbeableConnector(pluginName, config)) {
            try {
                probeJdbc(pluginName, config, classLoader, timeoutMillis);
                result.put("status", "success");
                result.put("probeMethod", "jdbc-driver");
                result.put("message", "Connection succeeded via JDBC driver.");
                return result;
            } catch (Exception e) {
                result.put("status", "failed");
                result.put("probeMethod", "jdbc-driver");
                result.put("message", firstLine(e.getMessage()));
                return result;
            }
        }

        // Tier 5: 通用 TCP 端口连通性探针 (因已通过快速预检，直接返回已验证)
        // Tier 5: Generic TCP Socket Probe (already verified via pre-flight handshake)
        if (endpoint != null) {
            Map<String, Object> res = new LinkedHashMap<>();
            res.put("status", "success");
            res.put("probeMethod", "tcp-port");
            res.put(
                    "message",
                    String.format(
                            "TCP network reachability verified (%s:%d).",
                            endpoint.host, endpoint.port));
            result.putAll(res);
            return result;
        }

        // Tier 6: 平台未覆盖专有探针，安全跳过作为 Info (不阻断任务校验整体判定)
        // Tier 6: Platform has no dedicated probe, safely skipped as Info (does not fail
        // validation)
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
     * 尝试通过 SeaTunnel 3.0+ 的 SupportSourceDryRunValidation / SupportSinkDryRunValidation 探测。 Try
     * probing via SeaTunnel 3.0+ SupportSourceDryRunValidation / SupportSinkDryRunValidation.
     */
    private static Map<String, Object> tryProbeOfficialDryRun(
            ProxyNode node, ReadonlyConfig readonlyConfig, ClassLoader classLoader) {
        try {
            if (node.getKind() == NodeKind.SOURCE) {
                Class<?> dryRunSourceClass;
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
                @SuppressWarnings("unchecked")
                Optional<?> optionalFactory =
                        (Optional<?>)
                                discoverMethod.invoke(
                                        null,
                                        classLoader,
                                        sourceFactoryClass,
                                        node.getPluginName());
                if (optionalFactory.isPresent()
                        && dryRunSourceClass.isInstance(optionalFactory.get())) {
                    Object factory = optionalFactory.get();
                    Class<?> ctxClass =
                            Class.forName(
                                    "org.apache.seatunnel.api.table.factory.TableSourceFactoryContext",
                                    false,
                                    classLoader);
                    Object ctx =
                            ctxClass.getConstructor(ReadonlyConfig.class, ClassLoader.class)
                                    .newInstance(readonlyConfig, classLoader);
                    Method inferMethod =
                            dryRunSourceClass.getMethod("inferSchemaForDryRun", ctxClass);
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
                }
            }
        } catch (Throwable t) {
            Throwable cause = t.getCause() != null ? t.getCause() : t;
            String msg = cause.getMessage();
            // 明确失败时输出错误，若是配置不兼容则降级
            if (msg != null
                    && (msg.contains("Connection")
                            || msg.contains("timeout")
                            || msg.contains("refused")
                            || msg.contains("Access denied"))) {
                Map<String, Object> failRes = new LinkedHashMap<>();
                failRes.put("status", "failed");
                failRes.put("probeMethod", "dry-run-spi");
                failRes.put("message", firstLine(msg));
                return failRes;
            }
        }
        return null;
    }

    /**
     * 尝试通过 SeaTunnel CatalogFactory 动态发现并打开 Catalog 进行连接探测。 Try discovering and opening Catalog via
     * SeaTunnel CatalogFactory for connectivity probe.
     */
    private static Map<String, Object> tryProbeCatalog(
            String pluginName, ReadonlyConfig readonlyConfig, ClassLoader classLoader) {
        String catalogIdentifier = pluginName;
        if (pluginName.endsWith("-CDC")) {
            // 对齐 SeaTunnel 官方 CatalogTableUtil 规范：剥离 -CDC 路由至底层数据库 Catalog
            // Align with SeaTunnel official CatalogTableUtil specification: strip -CDC to route to
            // base database catalog
            catalogIdentifier = pluginName.substring(0, pluginName.length() - 4);
        }
        try {
            Optional<Catalog> optionalCatalog =
                    FactoryUtil.createOptionalCatalog(
                            "stx_probe_" + catalogIdentifier,
                            readonlyConfig,
                            classLoader,
                            catalogIdentifier);
            if (optionalCatalog.isPresent()) {
                try (Catalog catalog = optionalCatalog.get()) {
                    catalog.open();
                    Map<String, Object> res = new LinkedHashMap<>();
                    res.put("status", "success");
                    res.put("probeMethod", "catalog-spi");
                    res.put(
                            "message",
                            "Connected successfully via " + catalogIdentifier + " Catalog.");
                    return res;
                }
            }
        } catch (Throwable t) {
            String msg = t.getMessage();
            if (msg != null
                    && (msg.contains("Connection")
                            || msg.contains("Access denied")
                            || msg.contains("timeout")
                            || msg.contains("refused")
                            || msg.contains("Communications link failure"))) {
                Map<String, Object> failRes = new LinkedHashMap<>();
                failRes.put("status", "failed");
                failRes.put("probeMethod", "catalog-spi");
                failRes.put("message", firstLine(msg));
                return failRes;
            }
        }
        return null;
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
        return StringUtils.isNotBlank(url) && url.toLowerCase().startsWith("jdbc:");
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
            }
        }
        return defaultDb;
    }

    /** 执行 JDBC 驱动物理建连探测。 Probe JDBC connection via driver directly. */
    static void probeJdbc(String pluginName, Config config, ClassLoader classLoader)
            throws Exception {
        probeJdbc(pluginName, config, classLoader, 3500);
    }

    static void probeJdbc(
            String pluginName, Config config, ClassLoader classLoader, int timeoutMillis)
            throws Exception {
        String url = getString(config, "url");
        if (StringUtils.isBlank(url)) {
            String hostname = getString(config, "hostname");
            if (StringUtils.isBlank(hostname)) {
                hostname = getString(config, "host");
            }
            if (StringUtils.isNotBlank(hostname)) {
                String port = getString(config, "port");
                if ("MySQL-CDC".equalsIgnoreCase(pluginName)) {
                    if (StringUtils.isBlank(port)) {
                        port = "3306";
                    }
                    url = String.format("jdbc:mysql://%s:%s/", hostname, port);
                } else if ("Postgres-CDC".equalsIgnoreCase(pluginName)) {
                    if (StringUtils.isBlank(port)) {
                        port = "5432";
                    }
                    String db = resolveDatabaseName(config, "postgres");
                    url = String.format("jdbc:postgresql://%s:%s/%s", hostname, port, db);
                } else if ("SqlServer-CDC".equalsIgnoreCase(pluginName)) {
                    if (StringUtils.isBlank(port)) {
                        port = "1433";
                    }
                    String db = resolveDatabaseName(config, "master");
                    url =
                            String.format(
                                    "jdbc:sqlserver://%s:%s;databaseName=%s", hostname, port, db);
                } else if ("Oracle-CDC".equalsIgnoreCase(pluginName)) {
                    if (StringUtils.isBlank(port)) {
                        port = "1521";
                    }
                    String db = resolveDatabaseName(config, "XE");
                    url = String.format("jdbc:oracle:thin:@//%s:%s/%s", hostname, port, db);
                }
            }
        }
        if (StringUtils.isBlank(url)) {
            throw new IllegalArgumentException(
                    "Missing required connection option: url or hostname.");
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
            DriverManager.setLoginTimeout(Math.max(1, timeoutMillis / 1000));
            try (Connection ignored = driver.connect(url, properties)) {
                if (ignored == null) {
                    throw new IllegalStateException("JDBC driver returned null connection.");
                }
            }
            return;
        }
        throw new IllegalArgumentException("Missing jdbc driver option.");
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
        try (Socket socket = new Socket()) {
            socket.connect(new InetSocketAddress(endpoint.host, endpoint.port), TCP_TIMEOUT_MS);
            res.put("status", "success");
            res.put("probeMethod", "tcp-port");
            res.put(
                    "message",
                    String.format(
                            "TCP network reachability verified (%s:%d).",
                            endpoint.host, endpoint.port));
            return res;
        } catch (Exception e) {
            res.put("status", "failed");
            res.put("probeMethod", "tcp-port");
            res.put(
                    "message",
                    String.format(
                            "TCP connection failed (%s:%d): %s",
                            endpoint.host, endpoint.port, firstLine(e.getMessage())));
            return res;
        }
    }

    private static class HostAndPort {
        final String host;
        final int port;

        HostAndPort(String host, int port) {
            this.host = host;
            this.port = port;
        }
    }

    private static int resolveDefaultJdbcPort(String url) {
        String lower = url.toLowerCase();
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
        return 3306;
    }

    private static HostAndPort extractEndpoint(Config config) {
        String url = getString(config, "url");
        if (StringUtils.isNotBlank(url)) {
            try {
                if (url.startsWith("jdbc:")) {
                    Pattern pattern = Pattern.compile("//([^/:?#]+)(?::([0-9]+))?");
                    Matcher matcher = pattern.matcher(url);
                    if (matcher.find()) {
                        String host = matcher.group(1);
                        String portStr = matcher.group(2);
                        int port =
                                StringUtils.isNotBlank(portStr)
                                        ? Integer.parseInt(portStr)
                                        : resolveDefaultJdbcPort(url);
                        return new HostAndPort(host, port);
                    }
                } else if (url.startsWith("http://") || url.startsWith("https://")) {
                    URI uri = URI.create(url);
                    int port =
                            uri.getPort() != -1
                                    ? uri.getPort()
                                    : (url.startsWith("https") ? 443 : 80);
                    if (StringUtils.isNotBlank(uri.getHost())) {
                        return new HostAndPort(uri.getHost(), port);
                    }
                }
            } catch (Exception ignored) {
            }
        }
        String host = getString(config, "hostname");
        if (StringUtils.isBlank(host)) {
            host = getString(config, "host");
        }
        if (StringUtils.isNotBlank(host)) {
            String portStr = getString(config, "port");
            int port = 80;
            if (StringUtils.isNotBlank(portStr)) {
                try {
                    port = Integer.parseInt(portStr);
                } catch (NumberFormatException ignored) {
                }
            }
            return new HostAndPort(host, port);
        }
        String bootstrap = getString(config, "bootstrap.servers");
        if (StringUtils.isNotBlank(bootstrap)) {
            String first = bootstrap.split(",")[0].trim();
            HostAndPort hp = parseHostAndPortString(first, 9092);
            if (hp != null) {
                return hp;
            }
        }
        // Doris: 提取 fenodes (如 "127.0.0.1:8030")
        // Doris: Extract fenodes (e.g. "127.0.0.1:8030")
        String fenodes = getString(config, "fenodes");
        if (StringUtils.isNotBlank(fenodes)) {
            String first = fenodes.split(",")[0].trim();
            HostAndPort hp = parseHostAndPortString(first, 8030);
            if (hp != null) {
                return hp;
            }
        }
        // Hive: 提取 metastore_uri / hive.metastore.uris (如 "thrift://127.0.0.1:9083")
        // Hive: Extract metastore_uri / hive.metastore.uris (e.g. "thrift://127.0.0.1:9083")
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
        // S3 / MinIO / OSS: Extract fs.s3a.endpoint / s3.endpoint / endpoint
        String s3Endpoint = getString(config, "fs.s3a.endpoint");
        if (StringUtils.isBlank(s3Endpoint)) {
            s3Endpoint = getString(config, "s3.endpoint");
        }
        if (StringUtils.isBlank(s3Endpoint)) {
            s3Endpoint = getString(config, "endpoint");
        }
        if (StringUtils.isNotBlank(s3Endpoint)) {
            String ep = s3Endpoint.trim();
            if (ep.startsWith("http://") || ep.startsWith("https://")) {
                try {
                    URI uri = URI.create(ep);
                    int port =
                            uri.getPort() != -1
                                    ? uri.getPort()
                                    : (ep.startsWith("https") ? 443 : 80);
                    if (StringUtils.isNotBlank(uri.getHost())) {
                        return new HostAndPort(uri.getHost(), port);
                    }
                } catch (Exception ignored) {
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

    /**
     * 解析形如 host:port 的字符串，缺省端口时使用默认值。 Parses host:port string, falling back to default port when
     * omitted.
     */
    private static HostAndPort parseHostAndPortString(String hostPortStr, int defaultPort) {
        if (StringUtils.isBlank(hostPortStr)) {
            return null;
        }
        String cleaned = hostPortStr.trim();
        if (cleaned.contains("/")) {
            cleaned = cleaned.substring(0, cleaned.indexOf('/'));
        }
        String[] parts = cleaned.split(":");
        if (parts.length >= 2) {
            try {
                return new HostAndPort(parts[0].trim(), Integer.parseInt(parts[1].trim()));
            } catch (NumberFormatException ignored) {
            }
        } else if (parts.length == 1 && StringUtils.isNotBlank(parts[0])) {
            return new HostAndPort(parts[0].trim(), defaultPort);
        }
        return null;
    }

    private static String resolveDisplayTarget(Config config) {
        String target = getString(config, "url");
        if (StringUtils.isBlank(target)) {
            String host = getString(config, "hostname");
            if (StringUtils.isBlank(host)) {
                host = getString(config, "host");
            }
            if (StringUtils.isNotBlank(host)) {
                String port = getString(config, "port");
                target = port != null ? host + ":" + port : host;
            }
        }
        if (StringUtils.isBlank(target)) {
            String bootstrap = getString(config, "bootstrap.servers");
            if (StringUtils.isNotBlank(bootstrap)) {
                target = bootstrap;
            }
        }
        if (StringUtils.isBlank(target)) {
            String fenodes = getString(config, "fenodes");
            if (StringUtils.isNotBlank(fenodes)) {
                target = fenodes;
            }
        }
        if (StringUtils.isBlank(target)) {
            String metastoreUri = getString(config, "metastore_uri");
            if (StringUtils.isBlank(metastoreUri)) {
                metastoreUri = getString(config, "hive.metastore.uris");
            }
            if (StringUtils.isNotBlank(metastoreUri)) {
                target = metastoreUri;
            }
        }
        if (StringUtils.isBlank(target)) {
            String s3Endpoint = getString(config, "fs.s3a.endpoint");
            if (StringUtils.isBlank(s3Endpoint)) {
                s3Endpoint = getString(config, "s3.endpoint");
            }
            if (StringUtils.isBlank(s3Endpoint)) {
                s3Endpoint = getString(config, "endpoint");
            }
            if (StringUtils.isNotBlank(s3Endpoint)) {
                target = s3Endpoint;
            }
        }
        return target;
    }

    private static String resolveDefaultJdbcDriver(String pluginName, String url) {
        if ("MySQL-CDC".equalsIgnoreCase(pluginName)
                || (url != null && url.toLowerCase().contains("mysql"))) {
            return "com.mysql.cj.jdbc.Driver";
        }
        if (url != null && url.toLowerCase().contains("postgresql")) {
            return "org.postgresql.Driver";
        }
        return null;
    }

    private static String getString(Config config, String path) {
        return config.hasPath(path) ? config.getString(path) : null;
    }

    private static String firstLine(String message) {
        if (StringUtils.isBlank(message)) {
            return "unknown error";
        }
        int newline = message.indexOf('\n');
        return newline >= 0 ? message.substring(0, newline).trim() : message.trim();
    }
}
