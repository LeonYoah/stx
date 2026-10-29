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
import org.apache.seatunnel.shade.com.typesafe.config.ConfigFactory;
import org.apache.seatunnel.shade.com.typesafe.config.ConfigValueFactory;

import org.apache.seatunnel.api.configuration.ReadonlyConfig;
import org.apache.seatunnel.api.configuration.util.OptionRule;
import org.apache.seatunnel.api.table.catalog.Catalog;
import org.apache.seatunnel.api.table.factory.CatalogFactory;

import io.github.leonyoah.stx.proxy.model.NodeKind;
import io.github.leonyoah.stx.proxy.model.ProxyNode;

import java.io.File;
import java.lang.reflect.InvocationHandler;
import java.lang.reflect.Proxy;
import java.net.URL;
import java.net.URLClassLoader;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Collections;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.regex.Pattern;

/**
 * PR84 探测测试公共工具：构造节点/配置、注入可控的假 Catalog SPI。 Shared helpers for PR84 probe tests: nodes, configs and
 * a controllable fake Catalog SPI injected through an isolated classloader.
 */
final class ProbeTestSupport {

    /** 认证失败原因关键字。 Keywords that count as an auth-failure reason. */
    static final Pattern AUTH_REASON =
            Pattern.compile(
                    "(?i).*(access denied|auth|password|credential|login|forbidden|403|530|28000|28P01|SASL).*",
                    Pattern.DOTALL);

    static final String FAKE_IDENTIFIER = "StxFakeProbe";

    private ProbeTestSupport() {}

    static ProxyNode node(NodeKind kind, String pluginName) {
        return new ProxyNode(
                kind.name().toLowerCase() + "-0",
                kind,
                pluginName,
                0,
                Collections.emptyList(),
                kind == NodeKind.SOURCE ? "ds" : "",
                Collections.emptyList(),
                Collections.emptyMap(),
                Collections.emptyMap());
    }

    static Config config(String pluginName, Map<String, Object> options) {
        Config config =
                ConfigFactory.empty()
                        .withValue("plugin_name", ConfigValueFactory.fromAnyRef(pluginName));
        for (Map.Entry<String, Object> e : options.entrySet()) {
            // 与真实 HOCON 解析一致：点号 key 视为路径（bootstrap.servers -> bootstrap { servers }）。
            // Same as real HOCON parsing: dotted keys are paths.
            config = config.withValue(e.getKey(), ConfigValueFactory.fromAnyRef(e.getValue()));
        }
        return config;
    }

    static Map<String, Object> opts(Object... kv) {
        Map<String, Object> m = new java.util.LinkedHashMap<>();
        for (int i = 0; i < kv.length; i += 2) {
            m.put(String.valueOf(kv[i]), kv[i + 1]);
        }
        return m;
    }

    /**
     * 本地 TCP 服务：接受连接后立即关闭（端口"可达"但不说任何协议）。 Local TCP server that accepts and immediately closes
     * connections: the port is "reachable" but speaks no protocol.
     */
    static final class ClosingServer implements AutoCloseable {
        final java.net.ServerSocket server;
        final Thread acceptor;

        ClosingServer() throws java.io.IOException {
            server = new java.net.ServerSocket(0, 50, java.net.InetAddress.getLoopbackAddress());
            acceptor =
                    new Thread(
                            () -> {
                                while (!server.isClosed()) {
                                    try (java.net.Socket ignored = server.accept()) {
                                        // close immediately
                                    } catch (java.io.IOException ignored) {
                                        return;
                                    }
                                }
                            },
                            "probe-test-closing-server");
            acceptor.setDaemon(true);
            acceptor.start();
        }

        int port() {
            return server.getLocalPort();
        }

        @Override
        public void close() throws java.io.IOException {
            server.close();
        }
    }

    /** 本地 TCP 服务：从不 accept，也不回任何字节（黑洞）。 Local TCP server that never accepts nor replies. */
    static java.net.ServerSocket blackholeServer() throws java.io.IOException {
        return new java.net.ServerSocket(0, 50, java.net.InetAddress.getLoopbackAddress());
    }

    static boolean isAuthReason(Object message) {
        return message != null && AUTH_REASON.matcher(String.valueOf(message)).matches();
    }

    /** Fake Catalog 行为。 Behavior of the fake catalog's open(). */
    interface OpenBehavior {
        void open() throws Exception;
    }

    static final Map<String, OpenBehavior> BEHAVIORS = new ConcurrentHashMap<>();
    static final AtomicInteger OPEN_CALLS = new AtomicInteger();

    /**
     * 创建只额外暴露 Fake CatalogFactory SPI 描述文件的隔离类加载器。 Creates an isolated classloader that only adds a
     * services file registering {@link FakeProbeCatalogFactory}.
     */
    static URLClassLoader fakeCatalogClassLoader() throws Exception {
        Path dir = Files.createTempDirectory("stx-probe-spi");
        Path services = dir.resolve("META-INF/services");
        Files.createDirectories(services);
        Files.write(
                services.resolve("org.apache.seatunnel.api.table.factory.Factory"),
                FakeProbeCatalogFactory.class.getName().getBytes(StandardCharsets.UTF_8));
        dir.toFile().deleteOnExit();
        return new URLClassLoader(
                new URL[] {dir.toUri().toURL()}, ProbeTestSupport.class.getClassLoader());
    }

    static File tmp() {
        return new File(System.getProperty("java.io.tmpdir"));
    }

    /** 可控 Fake CatalogFactory。 Controllable fake CatalogFactory (identifier StxFakeProbe). */
    public static class FakeProbeCatalogFactory implements CatalogFactory {
        @Override
        public String factoryIdentifier() {
            return FAKE_IDENTIFIER;
        }

        @Override
        public OptionRule optionRule() {
            return OptionRule.builder().build();
        }

        @Override
        public Catalog createCatalog(String catalogName, ReadonlyConfig options) {
            Object key = options.toMap().get("fake_behavior");
            OpenBehavior behavior = BEHAVIORS.get(String.valueOf(key));
            InvocationHandler handler =
                    (proxy, method, args) -> {
                        switch (method.getName()) {
                            case "open":
                                OPEN_CALLS.incrementAndGet();
                                if (behavior != null) {
                                    behavior.open();
                                }
                                return null;
                            case "close":
                                return null;
                            case "name":
                                return catalogName;
                            case "toString":
                                return "FakeProbeCatalog(" + key + ")";
                            case "hashCode":
                                return System.identityHashCode(proxy);
                            case "equals":
                                return proxy == args[0];
                            default:
                                throw new UnsupportedOperationException(method.getName());
                        }
                    };
            return (Catalog)
                    Proxy.newProxyInstance(
                            Catalog.class.getClassLoader(),
                            new Class<?>[] {Catalog.class},
                            handler);
        }
    }
}
