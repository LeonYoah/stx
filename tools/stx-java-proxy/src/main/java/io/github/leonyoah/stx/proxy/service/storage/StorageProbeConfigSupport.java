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

import org.apache.seatunnel.shade.org.apache.commons.lang3.StringUtils;

import org.apache.hadoop.conf.Configuration;
import org.apache.hadoop.fs.Path;

import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.io.File;
import java.util.Map;

public final class StorageProbeConfigSupport {

    private static final String S3_STORAGE = "s3";
    private static final String OSS_STORAGE = "oss";
    private static final String HDFS_STORAGE = "hdfs";

    private StorageProbeConfigSupport() {}

    /**
     * 统一构建 Hadoop Configuration：先加载本地 hdfs_site_path，再叠加显式配置与 seatunnel.hadoop.* 覆盖项。 Builds a
     * shared Hadoop Configuration: loads local hdfs_site_path first, then overlays explicit config
     * entries and stripped seatunnel.hadoop.* keys.
     */
    public static Configuration buildHadoopConfiguration(Map<String, String> config) {
        Configuration conf = new Configuration(false);
        if (config == null || config.isEmpty()) {
            return conf;
        }
        String hdfsSitePath =
                firstNonBlank(
                        config.get("hdfs_site_path"),
                        config.get("seatunnel.hadoop.hdfs_site_path"));
        if (hdfsSitePath != null) {
            File siteFile = new File(hdfsSitePath);
            if (!siteFile.isFile()) {
                throw new ProxyException(
                        400, "Configured hdfs_site_path does not exist: " + hdfsSitePath);
            }
            // 使用 Path(URI) 确保按本地文件而非 classpath 资源加载 XML。
            // Use Path(URI) so Hadoop loads the XML from local disk instead of classpath.
            conf.addResource(new Path(siteFile.toURI()));
        }
        for (Map.Entry<String, String> entry : config.entrySet()) {
            String key = entry.getKey();
            String value = entry.getValue();
            if (key == null || value == null || value.trim().isEmpty()) {
                continue;
            }
            conf.set(key, value);
            if (key.startsWith("seatunnel.hadoop.")) {
                conf.set(key.substring("seatunnel.hadoop.".length()), value);
            }
        }
        String disableCache = firstNonBlank(config.get("disable.cache"));
        if (disableCache != null && conf.get("fs.hdfs.impl.disable.cache") == null) {
            conf.setBoolean("fs.hdfs.impl.disable.cache", Boolean.parseBoolean(disableCache));
        }
        return conf;
    }

    /**
     * 统一解析运行时存储目标路径，支持已带 scheme 的绝对路径与 hdfs-site.xml 中的 nameservices 兜底。 Resolves a runtime storage
     * Path, preserving already-qualified URIs and falling back to fs.defaultFS or dfs.nameservices
     * loaded from hdfs-site.xml.
     */
    public static Path resolveTargetPath(
            Configuration conf,
            Map<String, String> config,
            String storageType,
            String namespace,
            String requestedPath,
            String operationLabel) {
        String effectivePath = firstNonBlank(requestedPath, namespace);
        String normalizedPath = effectivePath == null ? namespace.trim() : effectivePath.trim();
        if (normalizedPath.contains("://")) {
            return new Path(normalizedPath);
        }
        if (S3_STORAGE.equals(storageType)) {
            String bucket =
                    firstNonBlank(
                            config.get("s3.bucket"),
                            config.get("fs.defaultFS"),
                            conf == null ? null : conf.get("fs.defaultFS"));
            if (bucket == null) {
                throw new ProxyException(
                        400, "S3 " + operationLabel + " requires s3.bucket or fs.defaultFS");
            }
            return new Path(
                    joinBucketAndNamespace(normalizeBucket(bucket, "s3a://"), normalizedPath));
        }
        if (OSS_STORAGE.equals(storageType)) {
            String bucket =
                    firstNonBlank(
                            config.get("oss.bucket"),
                            config.get("fs.defaultFS"),
                            conf == null ? null : conf.get("fs.defaultFS"));
            if (bucket == null) {
                throw new ProxyException(
                        400, "OSS " + operationLabel + " requires oss.bucket or fs.defaultFS");
            }
            return new Path(
                    joinBucketAndNamespace(normalizeBucket(bucket, "oss://"), normalizedPath));
        }
        String defaultFS =
                firstNonBlank(
                        config.get("fs.defaultFS"),
                        config.get("seatunnel.hadoop.fs.defaultFS"),
                        conf == null ? null : conf.get("fs.defaultFS"));
        if (defaultFS == null && HDFS_STORAGE.equals(storageType) && conf != null) {
            String nameServices = firstNonBlank(conf.get("dfs.nameservices"));
            if (nameServices != null) {
                String primaryService = nameServices.split(",")[0].trim();
                if (!primaryService.isEmpty()) {
                    defaultFS = "hdfs://" + primaryService;
                }
            }
        }
        if (defaultFS != null) {
            return new Path(new Path(defaultFS), normalizedPath);
        }
        return new Path(normalizedPath);
    }

    private static String normalizeBucket(String bucket, String scheme) {
        String trimmed = bucket.trim();
        if (trimmed.contains("://")) {
            return trimmed;
        }
        return scheme + trimmed.replaceFirst("^/+", "");
    }

    private static String joinBucketAndNamespace(String bucket, String namespace) {
        String normalizedBucket = bucket.replaceAll("/+$", "");
        String normalizedNamespace = namespace.replaceFirst("^/+", "");
        if (normalizedNamespace.isEmpty()) {
            return normalizedBucket;
        }
        return normalizedBucket + "/" + normalizedNamespace;
    }

    private static String firstNonBlank(String... values) {
        for (String value : values) {
            if (value != null && !value.trim().isEmpty()) {
                return value.trim();
            }
        }
        return null;
    }

    public static void validateCheckpointConfig(Map<String, String> config) {
        requireNonBlank(config, "namespace");
        String storageType = requireNonBlank(config, "storage.type");
        if (S3_STORAGE.equalsIgnoreCase(storageType)) {
            requireNonBlank(config, "s3.bucket");
        }
    }

    public static void validateIMapConfig(Map<String, Object> config) {
        requireNonBlank(config, "namespace");
        requireNonBlank(config, "businessName");
        requireNonBlank(config, "clusterName");
        String storageType = requireNonBlank(config, "storage.type");
        if (S3_STORAGE.equalsIgnoreCase(storageType)) {
            requireNonBlank(config, "s3.bucket");
        }
    }

    public static void applyCheckpointFastFailDefaults(Map<String, String> config) {
        String storageType = stringValue(config.get("storage.type"));
        if (HDFS_STORAGE.equalsIgnoreCase(storageType)) {
            fillDefaultFSFromHdfsSiteIfMissing(config);
            return;
        }
        if (!S3_STORAGE.equalsIgnoreCase(storageType)) {
            return;
        }
        config.putIfAbsent("fs.s3a.connection.establish.timeout", "5000");
        config.putIfAbsent("fs.s3a.connection.timeout", "10000");
        config.putIfAbsent("fs.s3a.attempts.maximum", "1");
        config.putIfAbsent("fs.s3a.retry.limit", "1");
        // 禁用 S3A FileSystem 缓存，避免同 bucket 更换/错误凭证时误命中旧连接。
        // Disable S3A FileSystem cache so rotated or invalid credentials on the same bucket are
        // verified fresh.
        config.putIfAbsent("fs.s3a.impl.disable.cache", "true");
    }

    public static void applyIMapFastFailDefaults(Map<String, Object> config, long timeoutMs) {
        String storageType = stringValue(config.get("storage.type"));
        if (HDFS_STORAGE.equalsIgnoreCase(storageType)) {
            fillDefaultFSFromHdfsSiteIfMissingObjectMap(config);
        }
        if (S3_STORAGE.equalsIgnoreCase(storageType)) {
            config.putIfAbsent("fs.s3a.connection.establish.timeout", "5000");
            config.putIfAbsent("fs.s3a.connection.timeout", "10000");
            config.putIfAbsent("fs.s3a.attempts.maximum", "1");
            config.putIfAbsent("fs.s3a.retry.limit", "1");
            config.putIfAbsent("fs.s3a.impl.disable.cache", "true");
        }
        config.putIfAbsent("writeDataTimeoutMilliseconds", Math.max(1000L, timeoutMs));
    }

    // 仅提供 hdfs_site_path 而未显式传 fs.defaultFS 时，从 XML 中推导 fs.defaultFS 满足 SeaTunnel HdfsConfiguration
    // 校验。
    // When hdfs_site_path is given without an explicit fs.defaultFS, derive fs.defaultFS from the
    // XML
    // so SeaTunnel's HdfsConfiguration checkConfiguration("fs.defaultFS") succeeds.
    private static void fillDefaultFSFromHdfsSiteIfMissing(Map<String, String> config) {
        if (firstNonBlank(config.get("fs.defaultFS"), config.get("seatunnel.hadoop.fs.defaultFS"))
                != null) {
            return;
        }
        if (firstNonBlank(
                        config.get("hdfs_site_path"), config.get("seatunnel.hadoop.hdfs_site_path"))
                == null) {
            return;
        }
        Configuration conf = buildHadoopConfiguration(config);
        String resolved = resolveDefaultFSFromHadoopConf(conf);
        if (resolved != null) {
            config.put("fs.defaultFS", resolved);
        }
    }

    private static void fillDefaultFSFromHdfsSiteIfMissingObjectMap(Map<String, Object> config) {
        if (firstNonBlank(
                        stringValue(config.get("fs.defaultFS")),
                        stringValue(config.get("seatunnel.hadoop.fs.defaultFS")))
                != null) {
            return;
        }
        if (firstNonBlank(
                        stringValue(config.get("hdfs_site_path")),
                        stringValue(config.get("seatunnel.hadoop.hdfs_site_path")))
                == null) {
            return;
        }
        Map<String, String> stringMap = new java.util.LinkedHashMap<>();
        for (Map.Entry<String, Object> entry : config.entrySet()) {
            if (entry.getKey() != null && entry.getValue() != null) {
                stringMap.put(entry.getKey(), String.valueOf(entry.getValue()));
            }
        }
        Configuration conf = buildHadoopConfiguration(stringMap);
        String resolved = resolveDefaultFSFromHadoopConf(conf);
        if (resolved != null) {
            config.put("fs.defaultFS", resolved);
        }
    }

    private static String resolveDefaultFSFromHadoopConf(Configuration conf) {
        if (conf == null) {
            return null;
        }
        String defaultFS = firstNonBlank(conf.get("fs.defaultFS"));
        if (defaultFS != null) {
            return defaultFS;
        }
        String nameServices = firstNonBlank(conf.get("dfs.nameservices"));
        if (nameServices != null) {
            String primaryService = nameServices.split(",")[0].trim();
            if (!primaryService.isEmpty()) {
                return "hdfs://" + primaryService;
            }
        }
        return null;
    }

    public static String checkpointTimeoutHint() {
        return "Check endpoint reachability, bucket existence, and whether the required storage jars are available from SEATUNNEL_HOME/lib or pluginJars.";
    }

    public static String imapTimeoutHint() {
        return "Check endpoint reachability, bucket existence, and whether the required imap or Hadoop jars are available from SEATUNNEL_HOME/lib or pluginJars.";
    }

    private static String requireNonBlank(Map<?, ?> config, String key) {
        Object value = config.get(key);
        if (value == null || StringUtils.isBlank(String.valueOf(value))) {
            throw new ProxyException(400, "Missing required config field: " + key);
        }
        return String.valueOf(value);
    }

    private static String stringValue(Object value) {
        return value == null ? null : String.valueOf(value);
    }
}
