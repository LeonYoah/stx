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
        Configuration conf = StorageProbeConfigSupport.buildHadoopConfiguration(config);
        Path targetPath =
                StorageProbeConfigSupport.resolveTargetPath(
                        conf, config, storageType, namespace, requestedPath, "list");
        // 按次创建独立 FileSystem 并及时关闭，避免常驻 proxy 跨配置/跨凭证复用旧连接缓存。
        // Create an isolated FileSystem per call and close it to avoid stale cross-credential cache
        // hits in the long-lived proxy.
        try (FileSystem fs = FileSystem.newInstance(targetPath.toUri(), conf)) {
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

    private String lower(String value) {
        if (value == null) {
            return null;
        }
        return value.trim().toLowerCase(Locale.ROOT);
    }
}
