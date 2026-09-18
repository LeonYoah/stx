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

import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.util.LinkedHashMap;
import java.util.Map;

public class CheckpointProbeServiceTest {

    private final CheckpointProbeService service = new CheckpointProbeService();

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
}
