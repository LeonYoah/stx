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
import org.junit.jupiter.api.Assumptions;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.LinkedHashMap;
import java.util.Map;

public class CheckpointProbeServiceTest {

    private final CheckpointProbeService service = new CheckpointProbeService();

    @TempDir Path tempDir;

    @Test
    public void testRejectLocalCheckpointStorageType() {
        Map<String, Object> request = new LinkedHashMap<>();
        request.put("plugin", "hdfs");
        request.put("mode", "read_write");

        Map<String, Object> config = new LinkedHashMap<>();
        config.put("storage.type", "local");
        config.put("namespace", "/tmp/checkpoint-probe");
        request.put("config", config);

        ProxyException exception =
                Assertions.assertThrows(ProxyException.class, () -> service.probe(request));
        Assertions.assertEquals(400, exception.getStatusCode());
        Assertions.assertTrue(exception.getMessage().contains("Local checkpoint storage"));
    }

    @Test
    public void testRejectMissingS3Bucket() {
        Map<String, Object> request = new LinkedHashMap<>();
        request.put("plugin", "hdfs");

        Map<String, Object> config = new LinkedHashMap<>();
        config.put("storage.type", "s3");
        config.put("namespace", "/checkpoint-probe");
        request.put("config", config);

        ProxyException exception =
                Assertions.assertThrows(ProxyException.class, () -> service.probe(request));
        Assertions.assertEquals(400, exception.getStatusCode());
        Assertions.assertTrue(exception.getMessage().contains("s3.bucket"));
    }

    @Test
    public void testSequentialProbesDoNotReuseStaticHdfsStorageSingleton() throws Exception {
        Path firstNamespace = Files.createDirectories(tempDir.resolve("ck-first"));

        Map<String, Object> firstRequest = new LinkedHashMap<>();
        firstRequest.put("plugin", "hdfs");
        firstRequest.put("mode", "read_write");
        Map<String, Object> firstConfig = new LinkedHashMap<>();
        firstConfig.put("storage.type", "hdfs");
        firstConfig.put("fs.defaultFS", "file:///");
        firstConfig.put("seatunnel.hadoop.fs.file.impl", "org.apache.hadoop.fs.LocalFileSystem");
        firstConfig.put("namespace", firstNamespace.toString());
        firstRequest.put("config", firstConfig);

        Map<String, Object> firstResponse;
        try {
            firstResponse = service.probe(firstRequest);
        } catch (ProxyException e) {
            Assumptions.assumeFalse(
                    e.getMessage() != null
                            && e.getMessage().contains("getSubject is not supported"),
                    "Current JDK does not support Hadoop 3.1 Subject.getSubject");
            throw e;
        }
        Assertions.assertEquals(Boolean.TRUE, firstResponse.get("ok"));
        Assertions.assertEquals(Boolean.TRUE, firstResponse.get("writable"));
        Assertions.assertEquals(Boolean.TRUE, firstResponse.get("readable"));

        // 第二次探测改为不可达的 S3 端点，必须真实校验失败而不能复用首次 HdfsStorage 单例。
        // Second probe points to an unreachable S3 endpoint and must fail instead of reusing the
        // first HdfsStorage singleton.
        Map<String, Object> secondRequest = new LinkedHashMap<>();
        secondRequest.put("plugin", "hdfs");
        secondRequest.put("mode", "read_write");
        secondRequest.put("timeoutMs", 5000L);
        Map<String, Object> secondConfig = new LinkedHashMap<>();
        secondConfig.put("storage.type", "s3");
        secondConfig.put("s3.bucket", "s3a://non-existent-bucket");
        secondConfig.put("namespace", "/seatunnel/checkpoint/");
        secondConfig.put("fs.s3a.endpoint", "http://127.0.0.1:1");
        secondConfig.put("fs.s3a.path.style.access", "true");
        secondConfig.put("fs.s3a.access.key", "wrong-ak");
        secondConfig.put("fs.s3a.secret.key", "wrong-sk");
        secondConfig.put(
                "fs.s3a.aws.credentials.provider",
                "org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider");
        secondRequest.put("config", secondConfig);

        ProxyException exception =
                Assertions.assertThrows(ProxyException.class, () -> service.probe(secondRequest));
        Assertions.assertEquals(500, exception.getStatusCode());
    }

    @Test
    public void testRuntimeStorageServicesUseIsolatedFileSystem() throws Exception {
        Path root = Files.createDirectories(tempDir.resolve("runtime-fs"));
        Path sampleFile = root.resolve("sample.txt");
        Files.write(sampleFile, "hello-runtime-storage".getBytes(StandardCharsets.UTF_8));

        Map<String, Object> config = new LinkedHashMap<>();
        config.put("storage.type", "hdfs");
        config.put("fs.defaultFS", "file:///");
        config.put("namespace", root.toString());

        Map<String, Object> statRequest = new LinkedHashMap<>();
        statRequest.put("config", config);
        Map<String, Object> statResp;
        try {
            statResp = new RuntimeStorageStatService().stat(statRequest);
        } catch (ProxyException e) {
            Assumptions.assumeFalse(
                    e.getMessage() != null
                            && e.getMessage().contains("getSubject is not supported"),
                    "Current JDK does not support Hadoop 3.1 Subject.getSubject");
            throw e;
        }
        Assertions.assertEquals(Boolean.TRUE, statResp.get("exists"));
        Assertions.assertEquals(1L, statResp.get("fileCount"));

        Map<String, Object> listRequest = new LinkedHashMap<>();
        listRequest.put("config", config);
        Map<String, Object> listResp = new RuntimeStorageListService().list(listRequest);
        Assertions.assertEquals(Boolean.TRUE, listResp.get("ok"));

        Map<String, Object> previewRequest = new LinkedHashMap<>();
        previewRequest.put("config", config);
        previewRequest.put("path", sampleFile.toString());
        Map<String, Object> previewResp =
                new RuntimeStoragePreviewService().preview(previewRequest);
        Assertions.assertEquals("hello-runtime-storage", previewResp.get("textPreview"));
    }
}
