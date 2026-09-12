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

package io.github.leonyoah.stx.proxy;

import org.apache.seatunnel.common.utils.JsonUtils;

import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.LinkedHashMap;
import java.util.Map;

public class StxJavaProxyCliTest {

    @TempDir private Path tempDir;

    @Test
    public void testWritesProbeResponseToFile() throws Exception {
        Path requestFile = tempDir.resolve("request.json");
        Path responseFile = tempDir.resolve("response.json");
        Files.write(
                requestFile,
                "{\"config\":{\"storage.type\":\"hdfs\",\"namespace\":\"/tmp\"}}"
                        .getBytes(StandardCharsets.UTF_8));

        StxJavaProxyCli cli =
                new StxJavaProxyCli(
                        request -> {
                            Map<String, Object> response = new LinkedHashMap<>();
                            response.put("ok", true);
                            response.put("kind", "checkpoint");
                            response.put(
                                    "storageType",
                                    String.valueOf(
                                            ((Map<?, ?>) request.get("config"))
                                                    .get("storage.type")));
                            return response;
                        },
                        request -> {
                            throw new AssertionError("imap probe should not be invoked");
                        });

        ByteArrayOutputStream stdout = new ByteArrayOutputStream();
        ByteArrayOutputStream stderr = new ByteArrayOutputStream();
        int exitCode =
                cli.run(
                        new String[] {
                            "checkpoint",
                            "--request-file",
                            requestFile.toString(),
                            "--response-file",
                            responseFile.toString()
                        },
                        stdout,
                        stderr);

        Assertions.assertEquals(0, exitCode);
        Assertions.assertEquals("", stdout.toString(StandardCharsets.UTF_8.name()));
        Assertions.assertEquals("", stderr.toString(StandardCharsets.UTF_8.name()));

        @SuppressWarnings("unchecked")
        Map<String, Object> response =
                JsonUtils.parseObject(
                        new String(Files.readAllBytes(responseFile), StandardCharsets.UTF_8),
                        Map.class);
        Assertions.assertEquals(Boolean.TRUE, response.get("ok"));
        Assertions.assertEquals("checkpoint", response.get("kind"));
        Assertions.assertEquals("hdfs", response.get("storageType"));
    }

    @Test
    public void testWritesErrorJsonOnProbeFailure() throws Exception {
        Path responseFile = tempDir.resolve("error.json");
        StxJavaProxyCli cli =
                new StxJavaProxyCli(
                        request -> {
                            throw new ProxyException(504, "probe timeout");
                        },
                        request -> new LinkedHashMap<>());

        ByteArrayOutputStream stdout = new ByteArrayOutputStream();
        ByteArrayOutputStream stderr = new ByteArrayOutputStream();
        int exitCode =
                cli.run(
                        new String[] {
                            "checkpoint",
                            "--request-json",
                            "{\"config\":{}}",
                            "--response-file",
                            responseFile.toString()
                        },
                        stdout,
                        stderr);

        Assertions.assertEquals(1, exitCode);
        Assertions.assertTrue(
                stderr.toString(StandardCharsets.UTF_8.name()).contains("probe timeout"));

        @SuppressWarnings("unchecked")
        Map<String, Object> response =
                JsonUtils.parseObject(
                        new String(Files.readAllBytes(responseFile), StandardCharsets.UTF_8),
                        Map.class);
        Assertions.assertEquals(Boolean.FALSE, response.get("ok"));
        Assertions.assertEquals(504, ((Number) response.get("statusCode")).intValue());
        Assertions.assertEquals("probe timeout", response.get("message"));
    }

    @Test
    public void testRejectsMissingPayload() throws IOException {
        StxJavaProxyCli cli =
                new StxJavaProxyCli(
                        request -> new LinkedHashMap<>(), request -> new LinkedHashMap<>());

        ByteArrayOutputStream stdout = new ByteArrayOutputStream();
        ByteArrayOutputStream stderr = new ByteArrayOutputStream();
        int exitCode = cli.run(new String[] {"imap"}, stdout, stderr);

        Assertions.assertEquals(1, exitCode);
        Assertions.assertTrue(
                stderr.toString(StandardCharsets.UTF_8.name()).contains("Missing request payload"));
        Assertions.assertTrue(
                stdout.toString(StandardCharsets.UTF_8.name()).contains("\"ok\":false"));
    }
}
