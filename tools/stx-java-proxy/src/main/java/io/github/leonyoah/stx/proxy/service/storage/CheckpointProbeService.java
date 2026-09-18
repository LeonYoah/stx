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

import org.apache.seatunnel.engine.checkpoint.storage.PipelineState;
import org.apache.seatunnel.engine.checkpoint.storage.api.CheckpointStorage;
import org.apache.seatunnel.engine.checkpoint.storage.api.CheckpointStorageFactory;
import org.apache.seatunnel.engine.common.utils.FactoryUtil;

import io.github.leonyoah.stx.proxy.service.plugin.PluginClassLoaderUtils;
import io.github.leonyoah.stx.proxy.service.support.ProbeExecutionUtils;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.net.URLClassLoader;
import java.nio.charset.StandardCharsets;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;

public class CheckpointProbeService {

    public Map<String, Object> probe(Map<String, Object> request) {
        return ProbeExecutionUtils.runWithTimeout(
                request,
                "Checkpoint probe",
                StorageProbeConfigSupport.checkpointTimeoutHint(),
                () -> doProbe(request));
    }

    private Map<String, Object> doProbe(Map<String, Object> request) {
        String plugin = ProxyRequestUtils.getRequiredString(request, "plugin");
        if (!"hdfs".equalsIgnoreCase(plugin)) {
            throw new ProxyException(
                    400, "Unsupported checkpoint plugin: " + plugin + ", expected hdfs");
        }
        String mode = ProxyRequestUtils.getOptionalString(request, "mode");
        if (mode == null) {
            mode = "read_write";
        }
        List<String> pluginJars = ProxyRequestUtils.getStringList(request, "pluginJars");
        Map<String, String> config =
                ProxyRequestUtils.toStringMap(ProxyRequestUtils.getMap(request, "config"));
        String storageType = config.get("storage.type");
        validateStorageType(storageType);
        StorageProbeConfigSupport.validateCheckpointConfig(config);
        StorageProbeConfigSupport.applyCheckpointFastFailDefaults(config);

        StorageHandle<CheckpointStorage> storageHandle = createStorage(plugin, pluginJars, config);
        CheckpointStorage checkpointStorage = storageHandle.getStorage();

        Map<String, Object> response = new LinkedHashMap<>();
        response.put("ok", true);
        response.put("plugin", plugin);
        response.put("mode", mode);
        response.put("storageType", storageType);

        if ("init_only".equalsIgnoreCase(mode)) {
            response.put("initialized", true);
            storageHandle.close();
            return response;
        }
        if (!"read_write".equalsIgnoreCase(mode)) {
            storageHandle.close();
            throw new ProxyException(400, "Unsupported checkpoint probe mode: " + mode);
        }

        String jobId = "stx-java-proxy-job-" + UUID.randomUUID();
        PipelineState probeState =
                PipelineState.builder()
                        .jobId(jobId)
                        .pipelineId(1)
                        .checkpointId(System.currentTimeMillis())
                        .states("stx-java-proxy".getBytes(StandardCharsets.UTF_8))
                        .build();

        boolean readable = false;
        boolean writable = false;
        String storedCheckpoint = null;
        try {
            storedCheckpoint = checkpointStorage.storeCheckPoint(probeState);
            writable = true;
            List<PipelineState> latest = checkpointStorage.getLatestCheckpoint(jobId);
            readable = latest != null && !latest.isEmpty();
        } catch (Exception e) {
            throw new ProxyException(500, "Checkpoint probe failed: " + e.getMessage(), e);
        } finally {
            try {
                checkpointStorage.deleteCheckpoint(jobId);
            } catch (Exception ignored) {
                // ignore cleanup exceptions for probe result
            }
            storageHandle.close();
        }
        response.put("writable", writable);
        response.put("readable", readable);
        response.put("storedCheckpoint", storedCheckpoint);
        return response;
    }

    private void validateStorageType(String storageType) {
        if (storageType == null) {
            throw new ProxyException(
                    400, "Checkpoint probe requires remote config field: storage.type");
        }
        if ("local".equalsIgnoreCase(storageType) || "localfile".equalsIgnoreCase(storageType)) {
            throw new ProxyException(
                    400,
                    "Local checkpoint storage is intentionally disabled, use remote storage.type such as hdfs, s3, oss, or cos");
        }
    }

    private StorageHandle<CheckpointStorage> createStorage(
            String plugin, List<String> pluginJars, Map<String, String> config) {
        ClassLoader parent = Thread.currentThread().getContextClassLoader();
        URLClassLoader urlClassLoader = null;
        try {
            ClassLoader probeClassLoader = parent;
            if (!pluginJars.isEmpty()) {
                urlClassLoader = PluginClassLoaderUtils.createClassLoader(pluginJars, parent);
                probeClassLoader = urlClassLoader;
            }
            Thread currentThread = Thread.currentThread();
            ClassLoader originalClassLoader = currentThread.getContextClassLoader();
            currentThread.setContextClassLoader(probeClassLoader);
            try {
                CheckpointStorage storage =
                        FactoryUtil.discoverFactory(
                                        probeClassLoader, CheckpointStorageFactory.class, plugin)
                                .create(config);
                return new StorageHandle<>(storage, urlClassLoader);
            } finally {
                currentThread.setContextClassLoader(originalClassLoader);
            }
        } catch (Exception e) {
            PluginClassLoaderUtils.closeQuietly(urlClassLoader);
            throw new ProxyException(
                    500, "Failed to initialize checkpoint storage: " + e.getMessage(), e);
        }
    }

    private static final class StorageHandle<T> {
        private final T storage;
        private final URLClassLoader classLoader;

        private StorageHandle(T storage, URLClassLoader classLoader) {
            this.storage = storage;
            this.classLoader = classLoader;
        }

        private T getStorage() {
            return storage;
        }

        private void close() {
            PluginClassLoaderUtils.closeQuietly(classLoader);
        }
    }
}
