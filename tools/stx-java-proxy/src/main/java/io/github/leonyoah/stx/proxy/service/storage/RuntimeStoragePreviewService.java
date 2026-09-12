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
import java.net.URLClassLoader;
import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;

public class RuntimeStoragePreviewService {

    private static final int DEFAULT_MAX_BYTES = 64 * 1024;
    private static final int HEX_PREVIEW_LIMIT = 256;

    public Map<String, Object> preview(Map<String, Object> request) {
        return ProbeExecutionUtils.runWithTimeout(
                request,
                "Runtime storage preview",
                "Check storage endpoint, credentials, required jars, and file accessibility.",
                () -> doPreview(request));
    }

    private Map<String, Object> doPreview(Map<String, Object> request) throws IOException {
        int maxBytes = (int) ProxyRequestUtils.getLong(request, "maxBytes", DEFAULT_MAX_BYTES);
        if (maxBytes <= 0) {
            maxBytes = DEFAULT_MAX_BYTES;
        }

        String inlineContent = ProxyRequestUtils.getOptionalString(request, "contentBase64");
        String pathValue = ProxyRequestUtils.getOptionalString(request, "path");
        String fileName = ProxyRequestUtils.getOptionalString(request, "fileName");
        String storageType = null;
        byte[] bytes;
        long totalSize;

        if (inlineContent != null && !inlineContent.trim().isEmpty()) {
            bytes = decodeBase64(inlineContent);
            totalSize = bytes.length;
        } else {
            List<String> pluginJars = ProxyRequestUtils.getStringList(request, "pluginJars");
            Map<String, String> config =
                    ProxyRequestUtils.toStringMap(ProxyRequestUtils.getMap(request, "config"));
            storageType = lower(config.get("storage.type"));
            if (storageType == null) {
                throw new ProxyException(
                        400, "Runtime storage preview requires config.storage.type");
            }
            String namespace = config.get("namespace");
            if (namespace == null || namespace.trim().isEmpty()) {
                throw new ProxyException(400, "Runtime storage preview requires config.namespace");
            }
            if (pathValue == null || pathValue.trim().isEmpty()) {
                throw new ProxyException(400, "Runtime storage preview requires path");
            }
            FileReadResult readResult =
                    readWithFileSystem(
                            pluginJars, config, storageType, namespace, pathValue, maxBytes);
            bytes = readResult.bytes;
            totalSize = readResult.totalSize;
            pathValue = readResult.path;
            fileName = firstNonBlank(fileName, readResult.fileName);
        }

        if (fileName == null || fileName.trim().isEmpty()) {
            fileName = fileNameFromPath(pathValue);
        }
        Map<String, Object> response = new LinkedHashMap<>();
        response.put("ok", true);
        response.put("storageType", storageType);
        response.put("path", pathValue);
        response.put("fileName", fileName);
        response.put("sizeBytes", totalSize);
        response.put("truncated", totalSize > bytes.length);
        populateContentPreview(response, bytes);
        return response;
    }

    private FileReadResult readWithFileSystem(
            List<String> pluginJars,
            Map<String, String> config,
            String storageType,
            String namespace,
            String requestedPath,
            int maxBytes)
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
                    throw new ProxyException(
                            404, "Runtime storage file does not exist: " + targetPath);
                }
                FileStatus status = fs.getFileStatus(targetPath);
                if (status.isDirectory()) {
                    throw new ProxyException(
                            400, "Runtime storage preview requires a file path: " + targetPath);
                }
                try (FSDataInputStream in = fs.open(targetPath)) {
                    byte[] bytes = readAtMost(in, maxBytes);
                    return new FileReadResult(
                            bytes,
                            status.getLen(),
                            targetPath.toString(),
                            status.getPath().getName());
                }
            } finally {
                currentThread.setContextClassLoader(originalClassLoader);
            }
        } finally {
            PluginClassLoaderUtils.closeQuietly(urlClassLoader);
        }
    }

    private void populateContentPreview(Map<String, Object> response, byte[] bytes) {
        boolean binary = isBinary(bytes);
        response.put("binary", binary);
        response.put("encoding", binary ? "binary" : "utf-8");
        if (binary) {
            response.put("hexPreview", toHex(bytes, HEX_PREVIEW_LIMIT));
            response.put("textPreview", "");
            return;
        }
        response.put("textPreview", new String(bytes, StandardCharsets.UTF_8));
        response.put("hexPreview", "");
    }

    private byte[] readAtMost(FSDataInputStream in, int maxBytes) throws IOException {
        int initialCapacity = Math.max(1024, Math.min(maxBytes, 64 * 1024));
        ByteArrayOutputStream out = new ByteArrayOutputStream(initialCapacity);
        byte[] buffer = new byte[4096];
        int remaining = maxBytes;
        while (remaining > 0) {
            int read = in.read(buffer, 0, Math.min(buffer.length, remaining));
            if (read < 0) {
                break;
            }
            out.write(buffer, 0, read);
            remaining -= read;
        }
        return out.toByteArray();
    }

    byte[] loadRawBytes(Map<String, Object> request) throws IOException {
        int maxBytes = (int) ProxyRequestUtils.getLong(request, "maxBytes", Integer.MAX_VALUE);
        if (maxBytes <= 0) {
            maxBytes = Integer.MAX_VALUE;
        }

        String inlineContent = ProxyRequestUtils.getOptionalString(request, "contentBase64");
        String pathValue = ProxyRequestUtils.getOptionalString(request, "path");
        if (inlineContent != null && !inlineContent.trim().isEmpty()) {
            return decodeBase64(inlineContent);
        }

        List<String> pluginJars = ProxyRequestUtils.getStringList(request, "pluginJars");
        Map<String, String> config =
                ProxyRequestUtils.toStringMap(ProxyRequestUtils.getMap(request, "config"));
        String storageType = lower(config.get("storage.type"));
        if (storageType == null) {
            throw new ProxyException(400, "Runtime storage preview requires config.storage.type");
        }
        String namespace = config.get("namespace");
        if (namespace == null || namespace.trim().isEmpty()) {
            throw new ProxyException(400, "Runtime storage preview requires config.namespace");
        }
        if (pathValue == null || pathValue.trim().isEmpty()) {
            throw new ProxyException(400, "Runtime storage preview requires path");
        }
        return readWithFileSystem(pluginJars, config, storageType, namespace, pathValue, maxBytes)
                .bytes;
    }

    private byte[] decodeBase64(String content) {
        try {
            return Base64.getDecoder().decode(content);
        } catch (IllegalArgumentException e) {
            throw new ProxyException(400, "Invalid base64 content", e);
        }
    }

    private boolean isBinary(byte[] bytes) {
        if (bytes == null || bytes.length == 0) {
            return false;
        }
        int control = 0;
        for (byte value : bytes) {
            int current = value & 0xff;
            if (current == 0) {
                return true;
            }
            if ((current < 0x09 || (current > 0x0d && current < 0x20)) && current != 0x1b) {
                control++;
            }
        }
        return control > bytes.length / 8;
    }

    private String toHex(byte[] bytes, int limit) {
        int size = Math.min(bytes.length, limit);
        StringBuilder builder = new StringBuilder(size * 3);
        for (int i = 0; i < size; i++) {
            if (i > 0) {
                builder.append(' ');
            }
            builder.append(String.format("%02x", bytes[i] & 0xff));
        }
        if (bytes.length > limit) {
            builder.append(" …");
        }
        return builder.toString();
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
                throw new ProxyException(400, "S3 preview requires s3.bucket or fs.defaultFS");
            }
            return new Path(
                    joinBucketAndNamespace(normalizeBucket(bucket, "s3a://"), normalizedPath));
        }
        if ("oss".equals(storageType)) {
            String bucket = firstNonBlank(config.get("oss.bucket"), config.get("fs.defaultFS"));
            if (bucket == null) {
                throw new ProxyException(400, "OSS preview requires oss.bucket or fs.defaultFS");
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

    private String fileNameFromPath(String path) {
        if (path == null || path.trim().isEmpty()) {
            return "";
        }
        int index = path.lastIndexOf('/');
        if (index < 0 || index == path.length() - 1) {
            return path;
        }
        return path.substring(index + 1);
    }

    private String lower(String value) {
        if (value == null) {
            return null;
        }
        return value.trim().toLowerCase(Locale.ROOT);
    }

    private static final class FileReadResult {
        private final byte[] bytes;
        private final long totalSize;
        private final String path;
        private final String fileName;

        private FileReadResult(byte[] bytes, long totalSize, String path, String fileName) {
            this.bytes = bytes;
            this.totalSize = totalSize;
            this.path = path;
            this.fileName = fileName;
        }
    }
}
