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
import org.apache.hadoop.fs.FileStatus;
import org.apache.hadoop.fs.FileSystem;
import org.apache.hadoop.fs.Path;

import io.github.leonyoah.stx.proxy.service.plugin.PluginClassLoaderUtils;
import io.github.leonyoah.stx.proxy.service.support.ProbeExecutionUtils;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.io.IOException;
import java.net.URLClassLoader;
import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.Date;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.TimeZone;

public class RuntimeStorageListService {

    public Map<String, Object> list(Map<String, Object> request) {
        return ProbeExecutionUtils.runWithTimeout(
                request,
                "Runtime storage list",
                "Check storage endpoint, credentials, required jars, and namespace accessibility.",
                () -> doList(request));
    }

    private Map<String, Object> doList(Map<String, Object> request) {
        List<String> pluginJars = ProxyRequestUtils.getStringList(request, "pluginJars");
        Map<String, String> config =
                ProxyRequestUtils.toStringMap(ProxyRequestUtils.getMap(request, "config"));
        String storageType = lower(config.get("storage.type"));
        if (storageType == null) {
            throw new ProxyException(400, "Runtime storage list requires config.storage.type");
        }
        String namespace = config.get("namespace");
        if (namespace == null || namespace.trim().isEmpty()) {
            throw new ProxyException(400, "Runtime storage list requires config.namespace");
        }
        String requestedPath = ProxyRequestUtils.getOptionalString(request, "path");
        boolean recursive = ProxyRequestUtils.getBoolean(request, "recursive", false);
        int limit = (int) ProxyRequestUtils.getLong(request, "limit", 200L);
        if (limit <= 0) {
            limit = 200;
        }

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
                return listWithFileSystem(
                        config, storageType, namespace, requestedPath, recursive, limit);
            } finally {
                currentThread.setContextClassLoader(originalClassLoader);
            }
        } catch (ProxyException e) {
            throw e;
        } catch (Exception e) {
            throw new ProxyException(500, "Runtime storage list failed: " + e.getMessage(), e);
        } finally {
            PluginClassLoaderUtils.closeQuietly(urlClassLoader);
        }
    }

    private Map<String, Object> listWithFileSystem(
            Map<String, String> config,
            String storageType,
            String namespace,
            String requestedPath,
            boolean recursive,
            int limit)
            throws IOException {
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
            throw new ProxyException(404, "Runtime storage path does not exist: " + targetPath);
        }
        List<Map<String, Object>> items = new ArrayList<>();
        collect(fs, targetPath, recursive, limit, items);

        Map<String, Object> response = new LinkedHashMap<>();
        response.put("ok", true);
        response.put("storageType", storageType);
        response.put("path", targetPath.toString());
        response.put("items", items);
        return response;
    }

    private void collect(
            FileSystem fs,
            Path targetPath,
            boolean recursive,
            int limit,
            List<Map<String, Object>> items)
            throws IOException {
        FileStatus[] children = fs.listStatus(targetPath);
        if (children == null) {
            return;
        }
        for (FileStatus child : children) {
            if (items.size() >= limit) {
                return;
            }
            items.add(toItem(fs, child));
            if (recursive && child.isDirectory()) {
                collect(fs, child.getPath(), true, limit, items);
            }
        }
    }

    private Map<String, Object> toItem(FileSystem fs, FileStatus status) throws IOException {
        Map<String, Object> item = new LinkedHashMap<>();
        item.put("path", status.getPath().toString());
        item.put("name", status.getPath().getName());
        item.put("directory", status.isDirectory());
        item.put(
                "sizeBytes",
                status.isFile() ? status.getLen() : directorySize(fs, status.getPath()));
        item.put("modifiedAt", formatTimestamp(status.getModificationTime()));
        return item;
    }

    private long directorySize(FileSystem fs, Path root) throws IOException {
        try {
            return fs.getContentSummary(root).getLength();
        } catch (UnsupportedOperationException ignored) {
            return walk(fs, root)[0];
        }
    }

    private long[] walk(FileSystem fs, Path root) throws IOException {
        long total = 0L;
        long count = 0L;
        FileStatus[] children = fs.listStatus(root);
        if (children == null) {
            return new long[] {0L, 0L};
        }
        for (FileStatus child : children) {
            if (child.isFile()) {
                total += child.getLen();
                count += 1L;
                continue;
            }
            long[] nested = walk(fs, child.getPath());
            total += nested[0];
            count += nested[1];
        }
        return new long[] {total, count};
    }

    private String formatTimestamp(long timestamp) {
        if (timestamp <= 0) {
            return "";
        }
        SimpleDateFormat format = new SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ssXXX");
        format.setTimeZone(TimeZone.getDefault());
        return format.format(new Date(timestamp));
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
                throw new ProxyException(400, "S3 list requires s3.bucket or fs.defaultFS");
            }
            return new Path(
                    joinBucketAndNamespace(normalizeBucket(bucket, "s3a://"), normalizedPath));
        }
        if ("oss".equals(storageType)) {
            String bucket = firstNonBlank(config.get("oss.bucket"), config.get("fs.defaultFS"));
            if (bucket == null) {
                throw new ProxyException(400, "OSS list requires oss.bucket or fs.defaultFS");
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
}
