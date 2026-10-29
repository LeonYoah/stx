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

import org.apache.seatunnel.api.table.catalog.exception.CatalogException;

import org.apache.kafka.common.errors.SaslAuthenticationException;

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.Arguments;
import org.junit.jupiter.params.provider.MethodSource;

import io.github.leonyoah.stx.proxy.model.NodeKind;

import java.net.URLClassLoader;
import java.sql.SQLException;
import java.util.Map;
import java.util.stream.Stream;

import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.FAKE_IDENTIFIER;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.config;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.node;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.opts;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * PR84：认证失败必须被判定为失败（带认证原因），且不能回落到 TCP 成功。 PR84: auth failures must be reported as failed with an
 * auth reason, and must never fall back to a TCP "success".
 */
class ConnectionProbeAuthClassificationTest {

    private ProbeTestSupport.ClosingServer server;
    private URLClassLoader classLoader;

    @BeforeEach
    void setUp() throws Exception {
        server = new ProbeTestSupport.ClosingServer();
        classLoader = ProbeTestSupport.fakeCatalogClassLoader();
    }

    @AfterEach
    void tearDown() throws Exception {
        server.close();
        classLoader.close();
        ProbeTestSupport.BEHAVIORS.clear();
    }

    static Stream<Arguments> authErrors() {
        return Stream.of(
                // 真实 MySqlCatalog.open() 会把 SQLException 包成 "Failed connecting to ... via JDBC."
                // Real AbstractJdbcCatalog.open() wraps the SQLException like this.
                Arguments.of(
                        "mysql-28000-wrapped",
                        (ProbeTestSupport.OpenBehavior)
                                () -> {
                                    throw new CatalogException(
                                            "Failed connecting to jdbc:mysql://127.0.0.1:3306 via JDBC.",
                                            new SQLException(
                                                    "Access denied for user 'bad'@'172.17.0.1' (using password: YES)",
                                                    "28000",
                                                    1045));
                                }),
                Arguments.of(
                        "mysql-28000-raw",
                        (ProbeTestSupport.OpenBehavior)
                                () -> {
                                    // Catalog.open() 只能抛非受检异常；保留原始 SQL 消息。
                                    // Catalog.open() can only throw unchecked; keep the SQL
                                    // message.
                                    SQLException e =
                                            new SQLException(
                                                    "Access denied for user 'bad'@'172.17.0.1' (using password: YES)",
                                                    "28000",
                                                    1045);
                                    throw new CatalogException(e.getMessage(), e);
                                }),
                Arguments.of(
                        "postgres-28P01-wrapped",
                        (ProbeTestSupport.OpenBehavior)
                                () -> {
                                    throw new CatalogException(
                                            "Failed connecting to jdbc:postgresql://127.0.0.1:5432/postgres via JDBC.",
                                            new SQLException(
                                                    "FATAL: password authentication failed for user \"bad\"",
                                                    "28P01"));
                                }),
                Arguments.of(
                        "postgres-28P01-raw",
                        (ProbeTestSupport.OpenBehavior)
                                () -> {
                                    SQLException e =
                                            new SQLException(
                                                    "FATAL: password authentication failed for user \"bad\"",
                                                    "28P01");
                                    throw new CatalogException(e.getMessage(), e);
                                }),
                Arguments.of(
                        "kafka-sasl",
                        (ProbeTestSupport.OpenBehavior)
                                () -> {
                                    throw new SaslAuthenticationException(
                                            "Authentication failed: Invalid username or password");
                                }),
                Arguments.of(
                        "s3-403",
                        (ProbeTestSupport.OpenBehavior)
                                () -> {
                                    throw new RuntimeException(
                                            "com.amazonaws.services.s3.model.AmazonS3Exception: The AWS Access Key Id you provided does not exist in our records. (Service: Amazon S3; Status Code: 403; Error Code: InvalidAccessKeyId)");
                                }),
                Arguments.of(
                        "ftp-530",
                        (ProbeTestSupport.OpenBehavior)
                                () -> {
                                    java.io.IOException e =
                                            new java.io.IOException("530 Login incorrect.");
                                    throw new RuntimeException(e.getMessage(), e);
                                }));
    }

    @ParameterizedTest(name = "[{index}] {0} via Tier-1 probe, kind={2}")
    @MethodSource("authErrorsWithKind")
    void authErrorMustFailWithAuthReasonAndNotFallBackToTcp(
            String key, ProbeTestSupport.OpenBehavior behavior, NodeKind kind) {
        ProbeTestSupport.BEHAVIORS.put(key, behavior);
        Map<String, Object> result =
                ConnectionProbeSupport.probeConnector(
                        node(kind, FAKE_IDENTIFIER),
                        config(
                                FAKE_IDENTIFIER,
                                opts(
                                        "host",
                                        "127.0.0.1",
                                        "port",
                                        String.valueOf(server.port()),
                                        "fake_behavior",
                                        key)),
                        classLoader,
                        3000);
        assertNotEquals(
                "tcp-port",
                result.get("probeMethod"),
                "auth error must not fall back to TCP probe; result=" + result);
        assertEquals("failed", result.get("status"), "result=" + result);
        assertTrue(
                ProbeTestSupport.isAuthReason(result.get("message")),
                "message must carry an auth reason; result=" + result);
    }

    static Stream<Arguments> authErrorsWithKind() {
        return authErrors()
                .flatMap(
                        a ->
                                Stream.of(NodeKind.SOURCE, NodeKind.SINK)
                                        .map(k -> Arguments.of(a.get()[0], a.get()[1], k)));
    }

    /**
     * 真实连接器配置（带错误凭据）指向一个"端口开放但不讲协议"的服务：探测器无法验证凭据，因此绝不能报告 success。 Real connector configs with wrong
     * credentials against a port-open/no-protocol server: the probe cannot have verified the
     * credentials, so it must not report success.
     */
    static Stream<Arguments> credentialedConnectors() {
        return Stream.of(
                Arguments.of(
                        "Kafka",
                        new Object[] {
                            "bootstrap.servers", "127.0.0.1:%PORT%",
                            "topic", "t",
                            "kafka.config.security.protocol", "SASL_PLAINTEXT",
                            "kafka.config.sasl.mechanism", "PLAIN",
                            "kafka.config.sasl.jaas.config",
                                    "org.apache.kafka.common.security.plain.PlainLoginModule required username=\"bad\" password=\"bad\";"
                        }),
                Arguments.of(
                        "S3File",
                        new Object[] {
                            "bucket", "s3a://b",
                            "fs.s3a.endpoint", "http://127.0.0.1:%PORT%",
                            "access_key", "bad",
                            "secret_key", "bad",
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
                            "bad",
                            "password",
                            "bad",
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
                            "bad",
                            "password",
                            "bad",
                            "path",
                            "/x"
                        }));
    }

    @ParameterizedTest(name = "[{index}] {0} with wrong creds must not be TCP-success")
    @MethodSource("credentialedConnectors")
    void credentialedConnectorMustNotReportSuccessFromTcpOnly(String plugin, Object[] kv) {
        Object[] resolved = kv.clone();
        for (int i = 0; i < resolved.length; i++) {
            resolved[i] =
                    String.valueOf(resolved[i]).replace("%PORT%", String.valueOf(server.port()));
        }
        for (NodeKind kind : new NodeKind[] {NodeKind.SOURCE, NodeKind.SINK}) {
            Map<String, Object> result =
                    ConnectionProbeSupport.probeConnector(
                            node(kind, plugin), config(plugin, opts(resolved)), classLoader, 3000);
            assertNotEquals(
                    "success",
                    result.get("status"),
                    kind
                            + " "
                            + plugin
                            + " reported success without verifying creds; result="
                            + result);
        }
    }
}
