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
import org.junit.jupiter.api.Test;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.EnumSource;
import org.testcontainers.containers.GenericContainer;
import org.testcontainers.containers.wait.strategy.Wait;

import io.github.leonyoah.stx.proxy.model.NodeKind;

import java.net.HttpURLConnection;
import java.net.URL;
import java.time.Duration;
import java.util.Map;

import static io.github.leonyoah.stx.proxy.service.config.ProbeITSupport.*;
import static io.github.leonyoah.stx.proxy.service.config.ProbeTestSupport.opts;
import static org.junit.jupiter.api.Assertions.assertEquals;

/**
 * PR84 Tier-1 IT：S3File + MinIO。官方 minio/minio 在本机无法拉取（Docker Hub 拒绝），改用本仓库 real E2E 同款镜像
 * pgsty/minio（见 frontend/scripts/e2e/run-real-installer.sh）。 S3File against MinIO; minio/minio cannot be
 * pulled here, so the same pgsty/minio image used by this repo's real E2E harness is used.
 */
class S3ConnectionProbeIT {

    static GenericContainer<?> s3;

    @BeforeAll
    static void start() {
        s3 =
                capped(
                        new GenericContainer<>("pgsty/minio:RELEASE.2026-08-04T00-00-00Z")
                                .withEnv("MINIO_ROOT_USER", "stxaccess")
                                .withEnv("MINIO_ROOT_PASSWORD", "stxsecret")
                                .withCommand("server", "/data")
                                .withExposedPorts(9000)
                                .waitingFor(
                                        Wait.forHttp("/minio/health/live")
                                                .forPort(9000)
                                                .forStatusCode(200)
                                                .withStartupTimeout(Duration.ofMinutes(2))));
        s3.start();
    }

    @AfterAll
    static void stop() {
        if (s3 != null) {
            s3.stop();
        }
    }

    private static String endpoint() {
        return "http://127.0.0.1:" + s3.getMappedPort(9000);
    }

    private static Map<String, Object> cfg(String endpoint, String secret) {
        return opts(
                "bucket", "s3a://stx",
                "path", "/probe",
                "file_format_type", "text",
                "fs.s3a.endpoint", endpoint,
                "fs.s3a.aws.credentials.provider",
                        "org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider",
                "access_key", "stxaccess",
                "secret_key", secret);
    }

    /** 环境自检：匿名访问被拒（403），说明鉴权生效。 Sanity: anonymous request is rejected with 403. */
    @Test
    void sanityAnonymousIsForbidden() throws Exception {
        HttpURLConnection c = (HttpURLConnection) new URL(endpoint() + "/").openConnection();
        assertEquals(403, c.getResponseCode());
    }

    @ParameterizedTest(name = "S3File {0} correct keys -> success")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void correctCreds(NodeKind kind) {
        assertSuccess(probe(kind, "S3File", cfg(endpoint(), "stxsecret")));
    }

    @ParameterizedTest(name = "S3File {0} wrong secret -> auth failure")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void wrongCreds(NodeKind kind) {
        assertAuthFailure(probe(kind, "S3File", cfg(endpoint(), "WRONG")));
    }

    @ParameterizedTest(name = "S3File {0} closed port -> fast failure")
    @EnumSource(value = NodeKind.class, names = {"SOURCE", "SINK"})
    void closedPortFails(NodeKind kind) throws Exception {
        assertFastFailure(probe(kind, "S3File", cfg("http://127.0.0.1:" + closedPort(), "stxsecret")));
    }
}
