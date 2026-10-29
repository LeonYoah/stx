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

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;

import io.github.leonyoah.stx.proxy.model.DatasetDag;
import io.github.leonyoah.stx.proxy.model.JobConfigContext;
import io.github.leonyoah.stx.proxy.model.NodeKind;
import io.github.leonyoah.stx.proxy.model.ProxyEdge;

import java.lang.ref.WeakReference;
import java.net.ServerSocket;
import java.net.URLClassLoader;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.FAKE_IDENTIFIER;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.config;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.node;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.opts;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * PR84：探测必须在 timeout+2s 内返回失败，且超时后不残留探测线程 / 类加载器。 PR84: a hanging probe must fail within timeout+2s
 * and leave no lingering probe thread or pinned classloader.
 */
class ConnectionProbeTimeoutTest {

    private static final int TIMEOUT_MS = 1500;
    private static final long GRACE_MS = 2000;

    private final CountDownLatch release = new CountDownLatch(1);
    private final List<AutoCloseable> closeables = new ArrayList<>();

    @AfterEach
    void tearDown() throws Exception {
        release.countDown();
        for (AutoCloseable c : closeables) {
            c.close();
        }
        ProbeTestSupport.BEHAVIORS.clear();
        // 给被释放的探测线程一点时间退出，避免影响后续测试。 Let released probe threads exit.
        Thread.sleep(300);
    }

    private void hangUntilReleased() throws InterruptedException {
        // 最长 60s，确保即便测试失败也不会永远挂住。 Bounded so a failing test never hangs forever.
        release.await(60, TimeUnit.SECONDS);
    }

    /** 直接调用 probeConnector：Tier-1 Catalog.open() 挂起。 Direct call with a hanging Catalog.open(). */
    @Test
    void hangingCatalogProbeReturnsFailureWithinTimeout() throws Exception {
        ProbeTestSupport.ClosingServer server = new ProbeTestSupport.ClosingServer();
        closeables.add(server);
        URLClassLoader cl = ProbeTestSupport.fakeCatalogClassLoader();
        closeables.add(cl);
        ProbeTestSupport.BEHAVIORS.put("hang", this::hangUntilReleased);

        AtomicReference<Map<String, Object>> result = new AtomicReference<>();
        Thread caller =
                new Thread(
                        () ->
                                result.set(
                                        ConnectionProbeSupport.probeConnector(
                                                node(NodeKind.SOURCE, FAKE_IDENTIFIER),
                                                config(
                                                        FAKE_IDENTIFIER,
                                                        opts(
                                                                "host", "127.0.0.1",
                                                                "port",
                                                                        String.valueOf(
                                                                                server.port()),
                                                                "fake_behavior", "hang")),
                                                cl,
                                                TIMEOUT_MS)),
                        "probe-test-caller");
        caller.setDaemon(true);
        long start = System.nanoTime();
        caller.start();
        caller.join(TIMEOUT_MS + GRACE_MS);
        long elapsed = TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start);
        assertTrue(
                !caller.isAlive(),
                "probeConnector(timeoutMillis="
                        + TIMEOUT_MS
                        + ") still blocked after "
                        + elapsed
                        + "ms (Catalog.open() has no timeout guard)");
        assertEquals("failed", result.get().get("status"), "result=" + result.get());
    }

    /**
     * JDBC 驱动连接到"接受 TCP 但永不应答"的端口（如端口被别的服务占用）。 JDBC driver against a port that accepts TCP but
     * never answers the handshake.
     */
    @Test
    void jdbcProbeAgainstSilentPortReturnsWithinTimeout() throws Exception {
        ServerSocket blackhole = ProbeTestSupport.blackholeServer();
        closeables.add(blackhole);
        AtomicReference<Map<String, Object>> result = new AtomicReference<>();
        Thread caller =
                new Thread(
                        () ->
                                result.set(
                                        ConnectionProbeSupport.probeConnector(
                                                node(NodeKind.SOURCE, "Jdbc"),
                                                config(
                                                        "Jdbc",
                                                        opts(
                                                                "url",
                                                                        "jdbc:mysql://127.0.0.1:"
                                                                                + blackhole
                                                                                        .getLocalPort()
                                                                                + "/test",
                                                                "driver",
                                                                        "com.mysql.cj.jdbc.Driver",
                                                                "user", "root",
                                                                "password", "x")),
                                                getClass().getClassLoader(),
                                                TIMEOUT_MS)),
                        "probe-test-jdbc-caller");
        caller.setDaemon(true);
        long start = System.nanoTime();
        caller.start();
        caller.join(TIMEOUT_MS + GRACE_MS);
        long elapsed = TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start);
        boolean alive = caller.isAlive();
        // 关闭黑洞端口让驱动退出。 Close blackhole so the driver thread unblocks.
        blackhole.close();
        assertTrue(
                !alive,
                "Jdbc(mysql) probe still blocked after "
                        + elapsed
                        + "ms with timeoutMillis="
                        + TIMEOUT_MS);
        assertEquals("failed", result.get().get("status"), "result=" + result.get());
    }

    /**
     * 通过 ConfigValidationService（生产入口）：结果需在时限内返回，且之后不应残留 stx-connection-probe 线程，SeaTunnel home
     * 类加载器应可被回收。 Through the production entry point: result returned in time, and afterwards no
     * lingering stx-connection-probe thread and the classloader is collectable.
     */
    @Test
    void serviceTimeoutLeavesNoLingeringThreadOrClassLoader() throws Exception {
        ProbeTestSupport.ClosingServer server = new ProbeTestSupport.ClosingServer();
        closeables.add(server);
        ProbeTestSupport.BEHAVIORS.put("hang-svc", this::hangUntilReleased);
        WeakReference<URLClassLoader> clRef;
        long elapsed;
        Map<String, Object> result;
        {
            URLClassLoader cl = ProbeTestSupport.fakeCatalogClassLoader();
            clRef = new WeakReference<>(cl);
            HangingJobConfigSupportService support =
                    new HangingJobConfigSupportService(server.port(), cl);
            Map<String, Object> request = new LinkedHashMap<>();
            request.put("content", "env {}\nsource {}\nsink {}");
            request.put("contentFormat", "hocon");
            request.put("testConnection", true);
            request.put("timeoutMillis", TIMEOUT_MS);
            long start = System.nanoTime();
            result = new ConfigValidationService(support).validate(request);
            elapsed = TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start);
            support.cl = null;
            cl = null;
        }
        assertTrue(
                elapsed <= TIMEOUT_MS + GRACE_MS, "validate() took " + elapsed + "ms > timeout+2s");
        assertEquals(Boolean.FALSE, result.get("ok"), "result=" + result);

        Thread.sleep(1000);
        List<String> lingering = new ArrayList<>();
        for (Map.Entry<Thread, StackTraceElement[]> e : Thread.getAllStackTraces().entrySet()) {
            for (StackTraceElement el : e.getValue()) {
                if (el.getClassName().contains("ConnectionProbeSupport")) {
                    lingering.add(e.getKey().getName() + " @ " + el);
                    break;
                }
            }
        }
        for (int i = 0; i < 10 && clRef.get() != null; i++) {
            System.gc();
            Thread.sleep(100);
        }
        boolean clCollected = clRef.get() == null;
        List<String> problems = new ArrayList<>();
        if (!lingering.isEmpty()) {
            problems.add("probe threads still running 1s after validate() returned: " + lingering);
        }
        if (!clCollected) {
            problems.add("SeaTunnel-home classloader still strongly reachable after timeout");
        }
        assertTrue(problems.isEmpty(), String.join(" | ", problems));
    }

    /**
     * 返回 1 个 Fake Source + 1 个 Console Sink 的上下文。 Context with one fake source and a Console sink.
     */
    static class HangingJobConfigSupportService extends JobConfigSupportService {
        private final int port;
        volatile URLClassLoader cl;

        HangingJobConfigSupportService(int port, URLClassLoader cl) {
            this.port = port;
            this.cl = cl;
        }

        @Override
        public JobConfigContext parseJobContext(
                Map<String, Object> request, boolean skipOfficialDisplayPaths) {
            Config source =
                    config(
                            FAKE_IDENTIFIER,
                            opts(
                                    "host", "127.0.0.1",
                                    "port", String.valueOf(port),
                                    "fake_behavior", "hang-svc",
                                    "plugin_output", "fake"));
            Config sink =
                    ConfigFactory.parseString(
                            "plugin_name = \"Console\"\nplugin_input = [\"fake\"]\n");
            return new JobConfigContext(
                    ConfigFactory.empty(),
                    Collections.singletonList(source),
                    Collections.emptyList(),
                    Collections.singletonList(sink),
                    true,
                    Collections.emptyList(),
                    new DatasetDag(
                            Arrays.asList(
                                    node(NodeKind.SOURCE, FAKE_IDENTIFIER),
                                    node(NodeKind.SINK, "Console")),
                            Collections.singletonList(
                                    new ProxyEdge("fake", "source-0", "sink-0"))));
        }

        @Override
        public JobConfigContext parseJobContext(Map<String, Object> request) {
            return parseJobContext(request, false);
        }

        @Override
        URLClassLoader createSeatunnelHomeClassLoader(ClassLoader parent) {
            URLClassLoader result = cl;
            cl = null;
            return result;
        }
    }
}
