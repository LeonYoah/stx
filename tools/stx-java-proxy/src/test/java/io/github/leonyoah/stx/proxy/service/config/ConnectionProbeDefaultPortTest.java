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

import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.Arguments;
import org.junit.jupiter.params.provider.MethodSource;

import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.util.stream.Stream;

import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.config;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.opts;
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;

/**
 * PR84：缺省端口必须按连接器协议推导；非 HTTP 连接器绝不能默认 80。 PR84: default ports must follow the connector protocol;
 * non-HTTP connectors must never default to port 80.
 */
class ConnectionProbeDefaultPortTest {

    static Stream<Arguments> cases() {
        return Stream.of(
                // plugin, options, expectedHost, expectedPort, isHttp
                Arguments.of("MySQL-CDC", opts("hostname", "db.local"), "db.local", 3306, false),
                Arguments.of("Postgres-CDC", opts("hostname", "db.local"), "db.local", 5432, false),
                Arguments.of(
                        "SqlServer-CDC", opts("hostname", "db.local"), "db.local", 1433, false),
                Arguments.of("Oracle-CDC", opts("hostname", "db.local"), "db.local", 1521, false),
                Arguments.of("FtpFile", opts("host", "ftp.local"), "ftp.local", 21, false),
                Arguments.of("SftpFile", opts("host", "sftp.local"), "sftp.local", 22, false),
                Arguments.of(
                        "Kafka",
                        opts("bootstrap.servers", "kafka.local"),
                        "kafka.local",
                        9092,
                        false),
                Arguments.of(
                        "Jdbc", opts("url", "jdbc:mysql://db.local/test"), "db.local", 3306, false),
                Arguments.of(
                        "Jdbc",
                        opts("url", "jdbc:postgresql://db.local/test"),
                        "db.local",
                        5432,
                        false),
                // SQLServer URL 使用 ';' 分隔属性。 SQLServer URLs use ';' separated properties.
                Arguments.of(
                        "Jdbc",
                        opts("url", "jdbc:sqlserver://db.local;databaseName=test"),
                        "db.local",
                        1433,
                        false),
                Arguments.of(
                        "Jdbc",
                        opts("url", "jdbc:sqlserver://db.local:1444;databaseName=test"),
                        "db.local",
                        1444,
                        false),
                // MySQL-CDC 官方配置使用 base-url。 Official MySQL-CDC config uses base-url.
                Arguments.of(
                        "MySQL-CDC",
                        opts("base-url", "jdbc:mysql://db.local:3307/test"),
                        "db.local",
                        3307,
                        false),
                Arguments.of(
                        "S3File",
                        opts("fs.s3a.endpoint", "http://minio.local"),
                        "minio.local",
                        80,
                        true),
                Arguments.of("Http", opts("url", "https://api.local/x"), "api.local", 443, true));
    }

    @ParameterizedTest(name = "[{index}] {0} {1} -> {2}:{3}")
    @MethodSource("cases")
    void defaultPortFollowsConnectorProtocol(
            String plugin,
            java.util.Map<String, Object> options,
            String expectedHost,
            int expectedPort,
            boolean http)
            throws Exception {
        Object endpoint = extractEndpoint(config(plugin, options));
        assertNotNull(endpoint, plugin + " " + options + ": no endpoint extracted");
        String host = (String) field(endpoint, "host");
        int port = (Integer) field(endpoint, "port");
        if (!http) {
            assertNotEquals(80, port, plugin + " " + options + " defaulted to HTTP port 80");
        }
        assertEquals(expectedHost, host, plugin + " " + options + " host");
        assertEquals(expectedPort, port, plugin + " " + options + " port");
    }

    private static Object extractEndpoint(Config config) throws Exception {
        Method m = ConnectionProbeSupport.class.getDeclaredMethod("extractEndpoint", Config.class);
        m.setAccessible(true);
        return m.invoke(null, config);
    }

    private static Object field(Object target, String name) throws Exception {
        Field f = target.getClass().getDeclaredField(name);
        f.setAccessible(true);
        return f.get(target);
    }
}
