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
import org.apache.seatunnel.engine.serializer.api.Serializer;
import org.apache.seatunnel.engine.serializer.protobuf.ProtoStuffSerializer;

import org.apache.hadoop.conf.Configuration;
import org.apache.hadoop.fs.FSDataInputStream;
import org.apache.hadoop.fs.FileStatus;
import org.apache.hadoop.fs.FileSystem;
import org.apache.hadoop.fs.Path;

import io.github.leonyoah.stx.proxy.service.plugin.PluginClassLoaderUtils;
import io.github.leonyoah.stx.proxy.service.support.ProbeExecutionUtils;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.lang.reflect.Method;
import java.net.URLClassLoader;
import java.util.ArrayList;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;

public class CheckpointDeserializeService {

    private static final String COMPLETED_CHECKPOINT_CLASS =
            "org.apache.seatunnel.engine.server.checkpoint.CompletedCheckpoint";

    private final RuntimeStoragePreviewService previewService = new RuntimeStoragePreviewService();
    private final Serializer serializer = new ProtoStuffSerializer();

    public Map<String, Object> inspect(Map<String, Object> request) {
        return ProbeExecutionUtils.runWithTimeout(
                request,
                "Checkpoint deserialize",
                "Check checkpoint file readability, required jars, and checkpoint serialization compatibility.",
                () -> doInspect(request));
    }

    private Map<String, Object> doInspect(Map<String, Object> request) throws IOException {
        Map<String, Object> preview = previewService.preview(request);
        byte[] rawBytes = loadRawBytes(request);
        PipelineState pipelineState = serializer.deserialize(rawBytes, PipelineState.class);
        Object checkpoint = deserializeCompletedCheckpoint(pipelineState.getStates());

        Map<String, Object> response = new LinkedHashMap<>(preview);
        response.put("pipelineState", buildPipelineState(pipelineState));
        response.put("completedCheckpoint", buildCompletedCheckpoint(checkpoint));
        response.put("actionStates", buildActionStates(asMap(invoke(checkpoint, "getTaskStates"))));
        response.put(
                "taskStatistics",
                buildTaskStatistics(asMap(invoke(checkpoint, "getTaskStatistics"))));
        return response;
    }

    private Object deserializeCompletedCheckpoint(byte[] bytes) throws IOException {
        try {
            Class<?> clazz = Class.forName(COMPLETED_CHECKPOINT_CLASS);
            return serializer.deserialize(bytes, clazz);
        } catch (ClassNotFoundException e) {
            throw new ProxyException(500, "CompletedCheckpoint class is unavailable", e);
        }
    }

    private byte[] loadRawBytes(Map<String, Object> request) throws IOException {
        String inlineContent = ProxyRequestUtils.getOptionalString(request, "contentBase64");
        if (inlineContent != null && !inlineContent.trim().isEmpty()) {
            return decodeBase64(inlineContent);
        }

        List<String> pluginJars = ProxyRequestUtils.getStringList(request, "pluginJars");
        Map<String, String> config =
                ProxyRequestUtils.toStringMap(ProxyRequestUtils.getMap(request, "config"));
        String storageType = lower(config.get("storage.type"));
        if (storageType == null) {
            throw new ProxyException(400, "Checkpoint deserialize requires config.storage.type");
        }
        String namespace = config.get("namespace");
        if (namespace == null || namespace.trim().isEmpty()) {
            throw new ProxyException(400, "Checkpoint deserialize requires config.namespace");
        }
        String pathValue = ProxyRequestUtils.getOptionalString(request, "path");
        if (pathValue == null || pathValue.trim().isEmpty()) {
            throw new ProxyException(400, "Checkpoint deserialize requires path");
        }
        return readWithFileSystem(pluginJars, config, storageType, namespace, pathValue);
    }

    private byte[] readWithFileSystem(
            List<String> pluginJars,
            Map<String, String> config,
            String storageType,
            String namespace,
            String requestedPath)
            throws IOException {
        ClassLoader parent = Thread.currentThread().getContextClassLoader();
        URLClassLoader urlClassLoader = null;
        try {
            ClassLoader runtimeClassLoader = parent;
            if (!pluginJars.isEmpty()) {
                urlClassLoader = PluginClassLoaderUtils.createClassLoader(pluginJars, parent);
                runtimeClassLoader = urlClassLoader;
            }
            Thread currentThread = Thread.currentThread();
            ClassLoader originalClassLoader = currentThread.getContextClassLoader();
            currentThread.setContextClassLoader(runtimeClassLoader);
            try {
                Configuration conf = new Configuration(false);
                for (Map.Entry<String, String> entry : config.entrySet()) {
                    String key = entry.getKey();
                    String value = entry.getValue();
                    if (value == null || value.trim().isEmpty()) {
                        continue;
                    }
                    conf.set(key, value);
                    if (key.startsWith("seatunnel.hadoop.")) {
                        conf.set(key.substring("seatunnel.hadoop.".length()), value);
                    }
                }
                Path targetPath = buildTargetPath(config, storageType, namespace, requestedPath);
                FileSystem fs = targetPath.getFileSystem(conf);
                if (!fs.exists(targetPath)) {
                    throw new ProxyException(404, "Checkpoint file does not exist: " + targetPath);
                }
                FileStatus status = fs.getFileStatus(targetPath);
                if (status.isDirectory()) {
                    throw new ProxyException(
                            400, "Checkpoint deserialize requires a file path: " + targetPath);
                }
                try (FSDataInputStream in = fs.open(targetPath)) {
                    return readAll(in);
                }
            } finally {
                currentThread.setContextClassLoader(originalClassLoader);
            }
        } finally {
            PluginClassLoaderUtils.closeQuietly(urlClassLoader);
        }
    }

    private byte[] readAll(FSDataInputStream in) throws IOException {
        byte[] buffer = new byte[4096];
        int bytesRead;
        try (FSDataInputStream stream = in;
                ByteArrayOutputStream out = new ByteArrayOutputStream()) {
            while ((bytesRead = stream.read(buffer)) >= 0) {
                out.write(buffer, 0, bytesRead);
            }
            return out.toByteArray();
        }
    }

    private byte[] decodeBase64(String content) {
        try {
            return Base64.getDecoder().decode(content);
        } catch (IllegalArgumentException e) {
            throw new ProxyException(400, "Invalid base64 content", e);
        }
    }

    private Path buildTargetPath(
            Map<String, String> config,
            String storageType,
            String namespace,
            String requestedPath) {
        String effectivePath = firstNonBlank(requestedPath, namespace);
        String normalizedPath = effectivePath == null ? namespace.trim() : effectivePath.trim();
        if ("s3".equals(storageType)) {
            String bucket = firstNonBlank(config.get("s3.bucket"), config.get("fs.defaultFS"));
            if (bucket == null) {
                throw new ProxyException(
                        400, "S3 checkpoint deserialize requires s3.bucket or fs.defaultFS");
            }
            return new Path(
                    joinBucketAndNamespace(normalizeBucket(bucket, "s3a://"), normalizedPath));
        }
        if ("oss".equals(storageType)) {
            String bucket = firstNonBlank(config.get("oss.bucket"), config.get("fs.defaultFS"));
            if (bucket == null) {
                throw new ProxyException(
                        400, "OSS checkpoint deserialize requires oss.bucket or fs.defaultFS");
            }
            return new Path(
                    joinBucketAndNamespace(normalizeBucket(bucket, "oss://"), normalizedPath));
        }
        String defaultFS =
                firstNonBlank(
                        config.get("fs.defaultFS"), config.get("seatunnel.hadoop.fs.defaultFS"));
        if (defaultFS != null) {
            return new Path(new Path(defaultFS), normalizedPath);
        }
        return new Path(normalizedPath);
    }

    private String normalizeBucket(String bucket, String scheme) {
        String trimmed = bucket.trim();
        if (trimmed.contains("://")) {
            return trimmed;
        }
        return scheme + trimmed.replaceFirst("^/+", "");
    }

    private String joinBucketAndNamespace(String bucket, String namespace) {
        String normalizedBucket = bucket.replaceAll("/+$", "");
        String normalizedNamespace = namespace.replaceFirst("^/+", "");
        if (normalizedNamespace.isEmpty()) {
            return normalizedBucket;
        }
        return normalizedBucket + "/" + normalizedNamespace;
    }

    private String firstNonBlank(String... values) {
        for (String value : values) {
            if (value != null && !value.trim().isEmpty()) {
                return value.trim();
            }
        }
        return null;
    }

    private String lower(String value) {
        if (value == null) {
            return null;
        }
        return value.trim().toLowerCase(Locale.ROOT);
    }

    private Map<String, Object> buildPipelineState(PipelineState state) {
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("jobId", state.getJobId());
        result.put("pipelineId", state.getPipelineId());
        result.put("checkpointId", state.getCheckpointId());
        result.put("stateBytes", state.getStates() == null ? 0 : state.getStates().length);
        return result;
    }

    private Map<String, Object> buildCompletedCheckpoint(Object checkpoint) {
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("jobId", invoke(checkpoint, "getJobId"));
        result.put("pipelineId", invoke(checkpoint, "getPipelineId"));
        result.put("checkpointId", invoke(checkpoint, "getCheckpointId"));
        Object checkpointType = invoke(checkpoint, "getCheckpointType");
        result.put(
                "checkpointType", checkpointType == null ? null : String.valueOf(checkpointType));
        result.put("triggerTimestamp", invoke(checkpoint, "getCheckpointTimestamp"));
        result.put("completedTimestamp", invoke(checkpoint, "getCompletedTimestamp"));
        result.put("restored", invoke(checkpoint, "isRestored"));
        result.put("taskStateCount", sizeOfMap(asMap(invoke(checkpoint, "getTaskStates"))));
        result.put(
                "taskStatisticsCount", sizeOfMap(asMap(invoke(checkpoint, "getTaskStatistics"))));
        return result;
    }

    private List<Map<String, Object>> buildActionStates(Map<?, ?> states) {
        List<Map<String, Object>> items = new ArrayList<>();
        if (states == null) {
            return items;
        }
        for (Map.Entry<?, ?> entry : states.entrySet()) {
            Object value = entry.getValue();
            Map<String, Object> item = new LinkedHashMap<>();
            item.put("name", invoke(entry.getKey(), "getName"));
            item.put("parallelism", invoke(value, "getParallelism"));
            item.put(
                    "coordinatorStateChunks",
                    stateChunkCount(invoke(value, "getCoordinatorState")));
            List<?> subtaskStates = asList(invoke(value, "getSubtaskStates"));
            item.put("subtaskCount", subtaskStates == null ? 0 : subtaskStates.size());
            List<Map<String, Object>> subtasks = new ArrayList<>();
            if (subtaskStates != null) {
                for (Object subtaskState : subtaskStates) {
                    if (subtaskState == null) {
                        continue;
                    }
                    Map<String, Object> subtask = new LinkedHashMap<>();
                    subtask.put("index", invoke(subtaskState, "getIndex"));
                    subtask.put("chunks", stateChunkCount(subtaskState));
                    subtask.put("bytes", stateChunkBytes(subtaskState));
                    subtasks.add(subtask);
                }
            }
            item.put("subtasks", subtasks);
            items.add(item);
        }
        return items;
    }

    private List<Map<String, Object>> buildTaskStatistics(Map<?, ?> statistics) {
        List<Map<String, Object>> items = new ArrayList<>();
        if (statistics == null) {
            return items;
        }
        for (Map.Entry<?, ?> entry : statistics.entrySet()) {
            Object value = entry.getValue();
            Map<String, Object> item = new LinkedHashMap<>();
            item.put("jobVertexId", entry.getKey());
            item.put("parallelism", invoke(value, "getParallelism"));
            item.put("acknowledgedSubtasks", invoke(value, "getNumAcknowledgedSubtasks"));
            item.put("completed", invoke(value, "isCompleted"));
            item.put("latestAckTimestamp", invoke(value, "getLatestAckTimestamp"));
            List<?> subtasksRaw = asList(invoke(value, "getSubtaskStats"));
            List<Map<String, Object>> subtasks = new ArrayList<>();
            if (subtasksRaw != null) {
                for (Object subtaskStatistics : subtasksRaw) {
                    if (subtaskStatistics == null) {
                        continue;
                    }
                    Map<String, Object> subtask = new LinkedHashMap<>();
                    subtask.put("subtaskIndex", invoke(subtaskStatistics, "getSubtaskIndex"));
                    subtask.put("ackTimestamp", invoke(subtaskStatistics, "getAckTimestamp"));
                    subtask.put("stateSize", invoke(subtaskStatistics, "getStateSize"));
                    Object status = invoke(subtaskStatistics, "getSubtaskStatus");
                    subtask.put("status", status == null ? null : String.valueOf(status));
                    subtasks.add(subtask);
                }
            }
            item.put("subtasks", subtasks);
            items.add(item);
        }
        return items;
    }

    private int stateChunkCount(Object state) {
        List<?> chunks = asList(invoke(state, "getState"));
        return chunks == null ? 0 : chunks.size();
    }

    private long stateChunkBytes(Object state) {
        List<?> chunks = asList(invoke(state, "getState"));
        if (chunks == null) {
            return 0L;
        }
        long total = 0L;
        for (Object chunk : chunks) {
            if (chunk instanceof byte[]) {
                total += ((byte[]) chunk).length;
            }
        }
        return total;
    }

    private int sizeOfMap(Map<?, ?> map) {
        return map == null ? 0 : map.size();
    }

    @SuppressWarnings("unchecked")
    private Map<?, ?> asMap(Object value) {
        if (value instanceof Map) {
            return (Map<?, ?>) value;
        }
        return null;
    }

    @SuppressWarnings("unchecked")
    private List<?> asList(Object value) {
        if (value instanceof List) {
            return (List<?>) value;
        }
        return null;
    }

    private Object invoke(Object target, String methodName) {
        if (target == null) {
            return null;
        }
        try {
            Method method = target.getClass().getMethod(methodName);
            return method.invoke(target);
        } catch (Exception e) {
            return null;
        }
    }
}
