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

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.Arguments;
import org.junit.jupiter.params.provider.MethodSource;

import io.github.leonyoah.stx.proxy.model.NodeKind;

import java.net.URLClassLoader;
import java.util.Map;
import java.util.stream.Stream;

import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.FAKE_IDENTIFIER;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.config;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.node;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.opts;
import static org.junit.jupiter.api.Assertions.assertNotEquals;

/**
 * PR84：每个 Sink 至少经过 Tier-1（Catalog/JDBC/DryRun），不能直接落到 TCP 探测。 PR84: every sink must go through at
 * least a Tier-1 protocol probe, never straight to TCP.
 */
class ConnectionProbeSinkTierTest {

    private ProbeTestSupport.ClosingServer server;
    private URLClassLoader classLoader;

    @BeforeEach
    void setUp() throws Exception {
        server = new ProbeTestSupport.ClosingServer();
        classLoader = ProbeTestSupport.fakeCatalogClassLoader();
        ProbeTestSupport.OPEN_CALLS.set(0);
    }

    @AfterEach
    void tearDown() throws Exception {
        server.close();
        classLoader.close();
    }

    static Stream<Arguments> sinks() {
        return Stream.of(
                Arguments.of(FAKE_IDENTIFIER, new Object[] {"host", "127.0.0.1", "port", "%PORT%"}),
                Arguments.of(
                        "Jdbc",
                        new Object[] {
                            "url", "jdbc:mysql://127.0.0.1:%PORT%/test",
                            "driver", "com.mysql.cj.jdbc.Driver",
                            "user", "root",
                            "password", "x"
                        }),
                Arguments.of(
                        "Kafka",
                        new Object[] {"bootstrap.servers", "127.0.0.1:%PORT%", "topic", "t"}),
                Arguments.of(
                        "S3File",
                        new Object[] {
                            "bucket", "s3a://b",
                            "fs.s3a.endpoint", "http://127.0.0.1:%PORT%",
                            "access_key", "a",
                            "secret_key", "b",
                            "path", "/x"
                        }),
                Arguments.of(
                        "FtpFile",
                        new Object[] {
                            "host",
                            "127.0.0.1",
                            "port",
                            "%PORT%",
                            "user",
                            "u",
                            "password",
                            "p",
                            "path",
                            "/x"
                        }),
                Arguments.of(
                        "SftpFile",
                        new Object[] {
                            "host",
                            "127.0.0.1",
                            "port",
                            "%PORT%",
                            "user",
                            "u",
                            "password",
                            "p",
                            "path",
                            "/x"
                        }));
    }

    @ParameterizedTest(name = "[{index}] Sink {0} uses a protocol-level tier")
    @MethodSource("sinks")
    void sinkGoesThroughProtocolTier(String plugin, Object[] kv) {
        Object[] resolved = kv.clone();
        for (int i = 0; i < resolved.length; i++) {
            resolved[i] =
                    String.valueOf(resolved[i]).replace("%PORT%", String.valueOf(server.port()));
        }
        Map<String, Object> result =
                ConnectionProbeSupport.probeConnector(
                        node(NodeKind.SINK, plugin),
                        config(plugin, opts(resolved)),
                        classLoader,
                        3000);
        assertNotEquals(
                "tcp-port",
                result.get("probeMethod"),
                "Sink " + plugin + " went straight to TCP probe; result=" + result);
    }
}
