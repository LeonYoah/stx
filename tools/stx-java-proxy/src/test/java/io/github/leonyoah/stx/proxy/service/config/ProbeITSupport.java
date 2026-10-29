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

import org.testcontainers.containers.GenericContainer;

import io.github.leonyoah.stx.proxy.model.NodeKind;

import java.net.ServerSocket;
import java.util.Map;
import java.util.concurrent.TimeUnit;

import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.config;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.node;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * PR84 Tier-1 Testcontainers IT 公共工具（容器限额 1GB / 2 CPU）。 Shared helpers for PR84 Tier-1 ITs; every
 * container is capped at 1GB RAM / 2 CPUs.
 */
final class ProbeITSupport {

    static final int TIMEOUT_MS = 3000;
    static final long GRACE_MS = 2000;
    static final long MEM_LIMIT = 1024L * 1024 * 1024;

    /**
     * 隔离的插件类加载器：target/it-plugins 下的 connector jar（如 connector-jdbc），父加载器为测试类路径（含驱动）。
     * Isolated plugin classloader over target/it-plugins (e.g. connector-jdbc), parent = test
     * classpath (JDBC drivers), mirroring the SEATUNNEL_HOME classloader used in production.
     */
    static final ClassLoader PLUGIN_CLASS_LOADER = buildPluginClassLoader();

    private ProbeITSupport() {}

    private static ClassLoader buildPluginClassLoader() {
        java.io.File dir = new java.io.File("target/it-plugins");
        java.io.File[] jars = dir.listFiles((d, n) -> n.endsWith(".jar"));
        if (jars == null || jars.length == 0) {
            throw new IllegalStateException(
                    "no connector jars in " + dir.getAbsolutePath() + " (run mvn generate-test-resources)");
        }
        java.net.URL[] urls = new java.net.URL[jars.length];
        try {
            for (int i = 0; i < jars.length; i++) {
                urls[i] = jars[i].toURI().toURL();
            }
        } catch (java.net.MalformedURLException e) {
            throw new IllegalStateException(e);
        }
        return new java.net.URLClassLoader(urls, ProbeITSupport.class.getClassLoader());
    }

    static <T extends GenericContainer<?>> T capped(T container) {
        container.withCreateContainerCmdModifier(
                cmd ->
                        cmd.getHostConfig()
                                .withMemory(MEM_LIMIT)
                                .withMemorySwap(MEM_LIMIT)
                                .withNanoCPUs(2_000_000_000L));
        return container;
    }

    /** 返回一个当前未监听的本地端口。 Returns a local port that nobody listens on. */
    static int closedPort() throws Exception {
        try (ServerSocket s = new ServerSocket(0)) {
            return s.getLocalPort();
        }
    }

    static Map<String, Object> probe(NodeKind kind, String plugin, Map<String, Object> options) {
        long start = System.nanoTime();
        Map<String, Object> result =
                ConnectionProbeSupport.probeConnector(
                        node(kind, plugin),
                        config(plugin, options),
                        PLUGIN_CLASS_LOADER,
                        TIMEOUT_MS);
        result.put(
                "_elapsedMs", TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start));
        System.out.println("[PR84-IT] " + kind + " " + plugin + " -> " + result);
        return result;
    }

    static void assertSuccess(Map<String, Object> r) {
        assertEquals("success", r.get("status"), "expected success; result=" + r);
    }

    static void assertAuthFailure(Map<String, Object> r) {
        assertNotEquals("tcp-port", r.get("probeMethod"), "auth check fell back to TCP; result=" + r);
        assertEquals("failed", r.get("status"), "wrong creds must fail; result=" + r);
        assertTrue(
                ProbeTestSupport.isAuthReason(r.get("message")),
                "failure message must carry an auth reason; result=" + r);
    }

    static void assertFastFailure(Map<String, Object> r) {
        assertEquals("failed", r.get("status"), "closed port must fail; result=" + r);
        long elapsed = (Long) r.get("_elapsedMs");
        assertTrue(
                elapsed <= TIMEOUT_MS + GRACE_MS,
                "closed port took " + elapsed + "ms > timeout+2s; result=" + r);
    }
}
