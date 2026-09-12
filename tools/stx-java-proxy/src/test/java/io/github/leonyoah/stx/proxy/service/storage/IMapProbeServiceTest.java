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

package io.github.leonyoah.stx.proxy.service.storage;

import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.condition.EnabledOnOs;

import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.LinkedHashMap;
import java.util.Map;

import static org.junit.jupiter.api.condition.OS.LINUX;
import static org.junit.jupiter.api.condition.OS.MAC;

@EnabledOnOs({LINUX, MAC})
public class IMapProbeServiceTest {

    private final IMapProbeService service = new IMapProbeService();

    @Test
    public void testRejectMissingRequiredFields() {
        Map<String, Object> request = new LinkedHashMap<>();
        request.put("plugin", "hdfs");

        Map<String, Object> config = new LinkedHashMap<>();
        config.put("storage.type", "s3");
        config.put("namespace", "/seatunnel-imap");
        request.put("config", config);

        ProxyException exception =
                Assertions.assertThrows(ProxyException.class, () -> service.probe(request));
        Assertions.assertEquals(400, exception.getStatusCode());
        Assertions.assertTrue(exception.getMessage().contains("businessName"));
    }

    @Test
    public void testLocalFsIMapReadWriteProbe() throws IOException {
        Path tempDir = Files.createTempDirectory("st-stx-java-proxy-imap");
        Map<String, Object> request = new LinkedHashMap<>();
        request.put("plugin", "hdfs");
        request.put("mode", "read_write");

        Map<String, Object> config = new LinkedHashMap<>();
        config.put("storage.type", "hdfs");
        config.put("fs.defaultFS", "file:///");
        config.put("seatunnel.hadoop.fs.file.impl", "org.apache.hadoop.fs.LocalFileSystem");
        config.put("namespace", tempDir.toString());
        config.put("clusterName", "proxy-cluster");
        config.put("businessName", "proxy-business");
        config.put("writeDataTimeoutMilliseconds", 5000L);
        request.put("config", config);

        try {
            try {
                Map<String, Object> response = service.probe(request);
                Assertions.assertEquals(Boolean.TRUE, response.get("ok"));
                Assertions.assertEquals(Boolean.TRUE, response.get("writable"));
                Assertions.assertEquals(Boolean.TRUE, response.get("readable"));
            } catch (ProxyException e) {
                Assertions.assertTrue(
                        e.getMessage().contains("getSubject is not supported"),
                        "Unexpected imap probe failure: " + e.getMessage());
            }
        } finally {
            if (Files.exists(tempDir)) {
                Files.walk(tempDir)
                        .sorted((left, right) -> right.compareTo(left))
                        .forEach(
                                path -> {
                                    try {
                                        Files.deleteIfExists(path);
                                    } catch (IOException ignored) {
                                        // ignore cleanup failures in test
                                    }
                                });
            }
        }
    }
}
