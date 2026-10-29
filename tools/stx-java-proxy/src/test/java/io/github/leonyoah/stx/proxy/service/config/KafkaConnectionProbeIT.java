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

import org.apache.kafka.clients.admin.AdminClient;
import org.apache.kafka.clients.admin.AdminClientConfig;
import org.apache.kafka.common.errors.SaslAuthenticationException;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.EnumSource;
import org.testcontainers.containers.FixedHostPortGenericContainer;
import org.testcontainers.containers.GenericContainer;
import org.testcontainers.containers.wait.strategy.Wait;
import org.testcontainers.images.builder.Transferable;

import io.github.leonyoah.stx.proxy.model.NodeKind;

import java.time.Duration;
import java.util.Map;
import java.util.Properties;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.TimeUnit;

import static io.github.leonyoah.stx.proxy.service.config.ProbeITSupport.*;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.opts;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * PR84 Tier-1 IT：apache/kafka:3.8.0 KRaft + SASL_PLAINTEXT/PLAIN，固定宿主 9092。 apache/kafka KRaft with
 * SASL_PLAINTEXT/PLAIN, fixed host port 9092.
 */
class KafkaConnectionProbeIT {

    static final String JAAS_OK =
            "org.apache.kafka.common.security.plain.PlainLoginModule required username=\"admin\" password=\"admin-secret\";";
    static final String JAAS_BAD =
            "org.apache.kafka.common.security.plain.PlainLoginModule required username=\"admin\" password=\"WRONG\";";

    static GenericContainer<?> kafka;

    @BeforeAll
    @SuppressWarnings("deprecation")
    static void start() {
        kafka =
                capped(
                        new FixedHostPortGenericContainer<>("apache/kafka:3.8.0")
                                .withFixedExposedPort(9092, 9092)
                                .withEnv("KAFKA_NODE_ID", "1")
                                .withEnv("KAFKA_PROCESS_ROLES", "broker,controller")
                                .withEnv("KAFKA_LISTENERS", "SASL_PLAINTEXT://:9092,CONTROLLER://:9093")
                                .withEnv("KAFKA_ADVERTISED_LISTENERS", "SASL_PLAINTEXT://localhost:9092")
                                .withEnv("KAFKA_CONTROLLER_LISTENER_NAMES", "CONTROLLER")
                                .withEnv(
                                        "KAFKA_LISTENER_SECURITY_PROTOCOL_MAP",
                                        "CONTROLLER:PLAINTEXT,SASL_PLAINTEXT:SASL_PLAINTEXT")
                                .withEnv("KAFKA_CONTROLLER_QUORUM_VOTERS", "1@localhost:9093")
                                .withEnv("KAFKA_INTER_BROKER_LISTENER_NAME", "SASL_PLAINTEXT")
                                .withEnv("KAFKA_SASL_ENABLED_MECHANISMS", "PLAIN")
                                .withEnv("KAFKA_SASL_MECHANISM_INTER_BROKER_PROTOCOL", "PLAIN")
                                .withEnv(
                                        "KAFKA_LISTENER_NAME_SASL__PLAINTEXT_PLAIN_SASL_JAAS_CONFIG",
                                        "org.apache.kafka.common.security.plain.PlainLoginModule required username=\"admin\" password=\"admin-secret\" user_admin=\"admin-secret\";")
                                .withEnv("KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR", "1")
                                .withEnv("KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR", "1")
                                .withEnv("KAFKA_TRANSACTION_STATE_LOG_MIN_ISR", "1")
                                .withEnv("KAFKA_HEAP_OPTS", "-Xmx512m -Xms256m")
                                // apache/kafka 的 SASL 配置脚本要求 KAFKA_OPTS 指向 JAAS 文件。
                                // The image's SASL configure step requires KAFKA_OPTS with a JAAS file.
                                .withCopyToContainer(
                                        Transferable.of(
                                                "KafkaServer {\n org.apache.kafka.common.security.plain.PlainLoginModule required username=\"admin\" password=\"admin-secret\" user_admin=\"admin-secret\";\n};\n"),
                                        "/etc/kafka/stx_jaas.conf")
                                .withEnv(
                                        "KAFKA_OPTS",
                                        "-Djava.security.auth.login.config=/etc/kafka/stx_jaas.conf")
                                .waitingFor(
                                        Wait.forLogMessage(".*Kafka Server started.*", 1)
                                                .withStartupTimeout(Duration.ofMinutes(2))));
        kafka.start();
    }

    @AfterAll
    static void stop() {
        if (kafka != null) {
            kafka.stop();
        }
    }

    private static Map<String, Object> cfg(String bootstrap, String jaas) {
        return opts(
                "bootstrap.servers", bootstrap,
                "topic", "stx_probe",
                "kafka.config.security.protocol", "SASL_PLAINTEXT",
                "kafka.config.sasl.mechanism", "PLAIN",
                "kafka.config.sasl.jaas.config", jaas);
    }

    /** 环境自检：确认 SASL 真的生效。 Environment sanity: SASL really enforced by the broker. */
    @Test
    void sanityBrokerEnforcesSasl() throws Exception {
        assertTrue(describe(JAAS_OK) > 0, "correct SASL creds should describe the cluster");
        ExecutionException e = assertThrows(ExecutionException.class, () -> describe(JAAS_BAD));
        assertTrue(
                e.getCause() instanceof SaslAuthenticationException,
                "wrong SASL creds should raise SaslAuthenticationException, got " + e.getCause());
    }

    private static int describe(String jaas) throws Exception {
        Properties p = new Properties();
        p.put(AdminClientConfig.BOOTSTRAP_SERVERS_CONFIG, "127.0.0.1:9092");
        p.put("security.protocol", "SASL_PLAINTEXT");
        p.put("sasl.mechanism", "PLAIN");
        p.put("sasl.jaas.config", jaas);
        p.put(AdminClientConfig.REQUEST_TIMEOUT_MS_CONFIG, "8000");
        p.put(AdminClientConfig.DEFAULT_API_TIMEOUT_MS_CONFIG, "10000");
        try (AdminClient admin = AdminClient.create(p)) {
            return admin.describeCluster().nodes().get(15, TimeUnit.SECONDS).size();
        }
    }

    @ParameterizedTest(name = "Kafka {0} correct SASL creds -> success")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void correctCreds(NodeKind kind) {
        assertSuccess(probe(kind, "Kafka", cfg("127.0.0.1:9092", JAAS_OK)));
    }

    @ParameterizedTest(name = "Kafka {0} wrong SASL creds -> auth failure")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void wrongCreds(NodeKind kind) {
        assertAuthFailure(probe(kind, "Kafka", cfg("127.0.0.1:9092", JAAS_BAD)));
    }

    @ParameterizedTest(name = "Kafka {0} closed port -> fast failure")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void closedPortFails(NodeKind kind) throws Exception {
        assertFastFailure(probe(kind, "Kafka", cfg("127.0.0.1:" + closedPort(), JAAS_OK)));
    }

    @ParameterizedTest(name = "Kafka {0} bootstrap without port -> default 9092 success")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void hostWithoutPort(NodeKind kind) {
        assertSuccess(probe(kind, "Kafka", cfg("127.0.0.1", JAAS_OK)));
    }
}
