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
import org.testcontainers.images.builder.Transferable;

import com.jcraft.jsch.JSch;
import com.jcraft.jsch.Session;
import io.github.leonyoah.stx.proxy.model.NodeKind;

import java.time.Duration;
import java.util.Map;
import java.util.Properties;

import static io.github.leonyoah.stx.proxy.service.config.ProbeITSupport.assertAuthFailure;
import static io.github.leonyoah.stx.proxy.service.config.ProbeITSupport.assertFastFailure;
import static io.github.leonyoah.stx.proxy.service.config.ProbeITSupport.assertSuccess;
import static io.github.leonyoah.stx.proxy.service.config.ProbeITSupport.capped;
import static io.github.leonyoah.stx.proxy.service.config.ProbeITSupport.closedPort;
import static io.github.leonyoah.stx.proxy.service.config.ProbeITSupport.probe;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.opts;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * PR84 Tier-1 IT：SftpFile (atmoz/sftp)，固定宿主 22。关闭 OpenSSH PerSourcePenalties，并在启动后真正完成一次密码登录，
 * 避免认证失败惩罚与“端口已监听但 sshd 未就绪”导致的 Connection reset。 SftpFile probe IT on fixed host port 22; disables
 * OpenSSH PerSourcePenalties and waits for a real password login.
 */
class SftpConnectionProbeIT {

    static GenericContainer<?> sftp;

    @BeforeAll
    @SuppressWarnings("deprecation")
    static void start() throws Exception {
        // Prefer disabling OpenSSH PerSourcePenalties (9.8+); fall back if the image's sshd
        // rejects the directive. 优先关闭 PerSourcePenalties；镜像 sshd 不识别该指令时回退。
        try {
            sftp = newSftpContainer(true);
            sftp.start();
        } catch (Exception first) {
            if (sftp != null) {
                try {
                    sftp.stop();
                } catch (Exception ignored) {
                    // no-op
                }
            }
            sftp = newSftpContainer(false);
            sftp.start();
        }
        waitForPasswordLogin("stx", "stxpw", 30_000);
    }

    @SuppressWarnings("deprecation")
    private static GenericContainer<?> newSftpContainer(boolean disablePenalties) {
        FixedHostPortGenericContainer<?> c =
                new FixedHostPortGenericContainer<>("atmoz/sftp:alpine")
                        .withFixedExposedPort(22, 22)
                        .withCommand("stx:stxpw:::upload")
                        .waitingFor(
                                Wait.forListeningPort().withStartupTimeout(Duration.ofMinutes(1)));
        if (disablePenalties) {
            c.withCopyToContainer(
                    Transferable.of("PerSourcePenalties no\n"),
                    "/etc/ssh/sshd_config.d/stx-no-penalties.conf");
        }
        return capped(c);
    }

    @AfterAll
    static void stop() {
        if (sftp != null) {
            sftp.stop();
        }
    }

    /** 轮询直到密码登录成功，确认 sshd 已可接受认证。 Poll until a password login succeeds. */
    private static void waitForPasswordLogin(String user, String password, long timeoutMs)
            throws Exception {
        long deadline = System.currentTimeMillis() + timeoutMs;
        Exception last = null;
        while (System.currentTimeMillis() < deadline) {
            Session session = null;
            try {
                JSch jsch = new JSch();
                session = jsch.getSession(user, "127.0.0.1", 22);
                session.setPassword(password);
                Properties props = new Properties();
                props.put("StrictHostKeyChecking", "no");
                props.put("PreferredAuthentications", "password,keyboard-interactive");
                session.setConfig(props);
                session.connect(3_000);
                assertTrue(session.isConnected(), "sftp session should be connected");
                return;
            } catch (Exception e) {
                last = e;
                Thread.sleep(500);
            } finally {
                if (session != null) {
                    try {
                        session.disconnect();
                    } catch (Exception ignored) {
                        // no-op
                    }
                }
            }
        }
        throw new IllegalStateException(
                "SFTP server did not accept password login within " + timeoutMs + "ms", last);
    }

    private static Map<String, Object> cfg(String host, Integer port, String password) {
        Map<String, Object> m =
                opts(
                        "host",
                        host,
                        "user",
                        "stx",
                        "password",
                        password,
                        "path",
                        "/",
                        "file_format_type",
                        "text");
        if (port != null) {
            m.put("port", String.valueOf(port));
        }
        return m;
    }

    @ParameterizedTest(name = "SftpFile {0} correct creds -> success")
    @EnumSource(
            value = NodeKind.class,
            names = {"SOURCE", "SINK"})
    void correctCreds(NodeKind kind) {
        assertSuccess(probe(kind, "SftpFile", cfg("127.0.0.1", 22, "stxpw")));
    }

    @ParameterizedTest(name = "SftpFile {0} closed port -> fast failure")
    @EnumSource(
            value = NodeKind.class,
            names = {"SOURCE", "SINK"})
    void closedPortFails(NodeKind kind) throws Exception {
        assertFastFailure(probe(kind, "SftpFile", cfg("127.0.0.1", closedPort(), "stxpw")));
    }

    @ParameterizedTest(name = "SftpFile {0} host without port -> default 22 success")
    @EnumSource(
            value = NodeKind.class,
            names = {"SOURCE", "SINK"})
    void hostWithoutPort(NodeKind kind) {
        assertSuccess(probe(kind, "SftpFile", cfg("127.0.0.1", null, "stxpw")));
    }

    @ParameterizedTest(name = "SftpFile {0} wrong creds -> auth failure")
    @EnumSource(
            value = NodeKind.class,
            names = {"SOURCE", "SINK"})
    void wrongCreds(NodeKind kind) {
        assertAuthFailure(probe(kind, "SftpFile", cfg("127.0.0.1", 22, "WRONG")));
    }
}
