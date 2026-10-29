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

import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.EnumSource;
import org.junit.jupiter.params.provider.ValueSource;
import org.testcontainers.containers.MySQLContainer;
import org.testcontainers.utility.DockerImageName;

import io.github.leonyoah.stx.proxy.model.NodeKind;

import java.util.Map;

import static io.github.leonyoah.stx.proxy.service.config.ProbeITSupport.*;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.opts;

/**
 * PR84 Tier-1 IT：mysql:8.0，覆盖 Jdbc(MySQL) Source/Sink 与 MySQL-CDC Source（base-url 与 hostname 两种写法）。
 * Covers Jdbc(MySQL) source/sink and MySQL-CDC source (official base-url form and hostname form).
 * 容器固定绑定宿主 3306，以覆盖"未写端口"用例。 Bound to host 3306 for the host-without-port case.
 */
class MySqlConnectionProbeIT {

    static FixedMySql mysql;

    static class FixedMySql extends MySQLContainer<FixedMySql> {
        FixedMySql() {
            super(DockerImageName.parse("mysql:8.0"));
            addFixedExposedPort(3306, 3306);
        }
    }

    @BeforeAll
    static void start() {
        mysql = capped(new FixedMySql()).withDatabaseName("test").withUsername("stx").withPassword("stxpw");
        mysql.start();
    }

    @AfterAll
    static void stop() {
        if (mysql != null) {
            mysql.stop();
        }
    }

    private static Map<String, Object> jdbc(String url, String password) {
        return opts(
                "url", url,
                "driver", "com.mysql.cj.jdbc.Driver",
                "user", "stx",
                "password", password,
                "query", "select 1",
                "database", "test",
                "table", "t");
    }

    private static Map<String, Object> cdc(String form, String host, Integer port, String password) {
        if ("base-url".equals(form)) {
            return opts(
                    "base-url",
                    "jdbc:mysql://" + host + (port == null ? "" : ":" + port) + "/test",
                    "username", "stx",
                    "password", password,
                    "database-names", java.util.Collections.singletonList("test"),
                    "table-names", java.util.Collections.singletonList("test.t"));
        }
        Map<String, Object> m =
                opts(
                        "hostname", host,
                        "username", "stx",
                        "password", password,
                        "database-names", java.util.Collections.singletonList("test"),
                        "table-names", java.util.Collections.singletonList("test.t"));
        if (port != null) {
            m.put("port", String.valueOf(port));
        }
        return m;
    }

    // ---------- Jdbc (MySQL) Source & Sink ----------

    @ParameterizedTest(name = "Jdbc-MySQL {0} correct creds -> success")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void jdbcCorrectCreds(NodeKind kind) {
        assertSuccess(probe(kind, "Jdbc", jdbc("jdbc:mysql://127.0.0.1:3306/test", "stxpw")));
    }

    @ParameterizedTest(name = "Jdbc-MySQL {0} wrong creds -> auth failure")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void jdbcWrongCreds(NodeKind kind) {
        assertAuthFailure(probe(kind, "Jdbc", jdbc("jdbc:mysql://127.0.0.1:3306/test", "WRONG")));
    }

    @ParameterizedTest(name = "Jdbc-MySQL {0} closed port -> fast failure")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void jdbcClosedPort(NodeKind kind) throws Exception {
        assertFastFailure(
                probe(kind, "Jdbc", jdbc("jdbc:mysql://127.0.0.1:" + closedPort() + "/test", "stxpw")));
    }

    @ParameterizedTest(name = "Jdbc-MySQL {0} url without port -> default 3306 success")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void jdbcHostWithoutPort(NodeKind kind) {
        assertSuccess(probe(kind, "Jdbc", jdbc("jdbc:mysql://127.0.0.1/test", "stxpw")));
    }

    // ---------- MySQL-CDC Source (source-only connector) ----------

    @ParameterizedTest(name = "MySQL-CDC SOURCE [{0}] correct creds -> success")
    @ValueSource(strings = {"base-url", "hostname"})
    void cdcCorrectCreds(String form) {
        assertSuccess(probe(NodeKind.SOURCE, "MySQL-CDC", cdc(form, "127.0.0.1", 3306, "stxpw")));
    }

    @ParameterizedTest(name = "MySQL-CDC SOURCE [{0}] wrong creds -> auth failure")
    @ValueSource(strings = {"base-url", "hostname"})
    void cdcWrongCreds(String form) {
        assertAuthFailure(probe(NodeKind.SOURCE, "MySQL-CDC", cdc(form, "127.0.0.1", 3306, "WRONG")));
    }

    @ParameterizedTest(name = "MySQL-CDC SOURCE [{0}] closed port -> fast failure")
    @ValueSource(strings = {"base-url", "hostname"})
    void cdcClosedPort(String form) throws Exception {
        assertFastFailure(
                probe(NodeKind.SOURCE, "MySQL-CDC", cdc(form, "127.0.0.1", closedPort(), "stxpw")));
    }

    @ParameterizedTest(name = "MySQL-CDC SOURCE [{0}] host without port -> default 3306 success")
    @ValueSource(strings = {"base-url", "hostname"})
    void cdcHostWithoutPort(String form) {
        assertSuccess(probe(NodeKind.SOURCE, "MySQL-CDC", cdc(form, "127.0.0.1", null, "stxpw")));
    }
}
