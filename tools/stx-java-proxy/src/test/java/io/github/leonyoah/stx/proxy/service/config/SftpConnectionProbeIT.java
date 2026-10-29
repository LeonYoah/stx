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
import org.testcontainers.containers.FixedHostPortGenericContainer;
import org.testcontainers.containers.GenericContainer;
import org.testcontainers.containers.wait.strategy.Wait;

import io.github.leonyoah.stx.proxy.model.NodeKind;

import java.time.Duration;
import java.util.Map;

import static io.github.leonyoah.stx.proxy.service.config.ProbeITSupport.*;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.opts;

/** PR84 Tier-1 IT：SftpFile (atmoz/sftp)，delfer/alpine-ftp-server，固定宿主 22。 SftpFile probe IT, fixed host port 22. */
class SftpConnectionProbeIT {

    static GenericContainer<?> sftp;

    @BeforeAll
    @SuppressWarnings("deprecation")
    static void start() {
        sftp =
                capped(
                        new FixedHostPortGenericContainer<>("atmoz/sftp:alpine")
                                .withFixedExposedPort(22, 22)
                                .withCommand("stx:stxpw:::upload")
                                .waitingFor(
                                        Wait.forListeningPort()
                                                .withStartupTimeout(Duration.ofMinutes(1))));
        sftp.start();
    }

    @AfterAll
    static void stop() {
        if (sftp != null) {
            sftp.stop();
        }
    }

    private static Map<String, Object> cfg(String host, Integer port, String password) {
        Map<String, Object> m =
                opts("host", host, "user", "stx", "password", password, "path", "/", "file_format_type", "text");
        if (port != null) {
            m.put("port", String.valueOf(port));
        }
        return m;
    }

    @ParameterizedTest(name = "SftpFile {0} correct creds -> success")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void correctCreds(NodeKind kind) {
        assertSuccess(probe(kind, "SftpFile", cfg("127.0.0.1", 22, "stxpw")));
    }

    @ParameterizedTest(name = "SftpFile {0} wrong creds -> auth failure")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void wrongCreds(NodeKind kind) {
        assertAuthFailure(probe(kind, "SftpFile", cfg("127.0.0.1", 22, "WRONG")));
    }

    @ParameterizedTest(name = "SftpFile {0} closed port -> fast failure")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void closedPortFails(NodeKind kind) throws Exception {
        assertFastFailure(probe(kind, "SftpFile", cfg("127.0.0.1", closedPort(), "stxpw")));
    }

    @ParameterizedTest(name = "SftpFile {0} host without port -> default 22 success")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void hostWithoutPort(NodeKind kind) {
        assertSuccess(probe(kind, "SftpFile", cfg("127.0.0.1", null, "stxpw")));
    }
}
