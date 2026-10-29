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

import org.apache.commons.net.ftp.FTPClient;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Test;
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
import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

/** PR84 Tier-1 IT：FtpFile，delfer/alpine-ftp-server，固定宿主 21。 FtpFile probe IT, fixed host port 21. */
class FtpConnectionProbeIT {

    static GenericContainer<?> ftp;

    @BeforeAll
    @SuppressWarnings("deprecation")
    static void start() {
        ftp =
                capped(
                        new FixedHostPortGenericContainer<>("delfer/alpine-ftp-server:latest")
                                .withFixedExposedPort(21, 21)
                                .withEnv("USERS", "stx|stxpw")
                                .withEnv("ADDRESS", "127.0.0.1")
                                .withEnv("MIN_PORT", "21000")
                                .withEnv("MAX_PORT", "21010")
                                .waitingFor(
                                        Wait.forListeningPort()
                                                .withStartupTimeout(Duration.ofMinutes(1))));
        ftp.start();
    }

    @AfterAll
    static void stop() {
        if (ftp != null) {
            ftp.stop();
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

    /** 环境自检：正确密码可登录，错误密码 530。 Sanity: correct password logs in, wrong one gets 530. */
    @Test
    void sanityServerEnforcesLogin() throws Exception {
        FTPClient ok = new FTPClient();
        ok.connect("127.0.0.1", 21);
        assertTrue(ok.login("stx", "stxpw"));
        ok.disconnect();
        FTPClient bad = new FTPClient();
        bad.connect("127.0.0.1", 21);
        assertFalse(bad.login("stx", "WRONG"));
        assertEquals(530, bad.getReplyCode());
        bad.disconnect();
    }

    @ParameterizedTest(name = "FtpFile {0} correct creds -> success")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void correctCreds(NodeKind kind) {
        assertSuccess(probe(kind, "FtpFile", cfg("127.0.0.1", 21, "stxpw")));
    }

    @ParameterizedTest(name = "FtpFile {0} wrong creds -> auth failure")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void wrongCreds(NodeKind kind) {
        assertAuthFailure(probe(kind, "FtpFile", cfg("127.0.0.1", 21, "WRONG")));
    }

    @ParameterizedTest(name = "FtpFile {0} closed port -> fast failure")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void closedPortFails(NodeKind kind) throws Exception {
        assertFastFailure(probe(kind, "FtpFile", cfg("127.0.0.1", closedPort(), "stxpw")));
    }

    @ParameterizedTest(name = "FtpFile {0} host without port -> default 21 success")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void hostWithoutPort(NodeKind kind) {
        assertSuccess(probe(kind, "FtpFile", cfg("127.0.0.1", null, "stxpw")));
    }
}
