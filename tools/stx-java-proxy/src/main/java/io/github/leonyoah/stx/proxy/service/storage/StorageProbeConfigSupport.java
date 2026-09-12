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

import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.util.Map;

public final class StorageProbeConfigSupport {

    private static final String S3_STORAGE = "s3";

    private StorageProbeConfigSupport() {}

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
        if (!S3_STORAGE.equalsIgnoreCase(storageType)) {
            return;
        }
        config.putIfAbsent("fs.s3a.connection.establish.timeout", "5000");
        config.putIfAbsent("fs.s3a.connection.timeout", "10000");
        config.putIfAbsent("fs.s3a.attempts.maximum", "1");
        config.putIfAbsent("fs.s3a.retry.limit", "1");
    }

    public static void applyIMapFastFailDefaults(Map<String, Object> config, long timeoutMs) {
        String storageType = stringValue(config.get("storage.type"));
        if (S3_STORAGE.equalsIgnoreCase(storageType)) {
            config.putIfAbsent("fs.s3a.connection.establish.timeout", "5000");
            config.putIfAbsent("fs.s3a.connection.timeout", "10000");
            config.putIfAbsent("fs.s3a.attempts.maximum", "1");
            config.putIfAbsent("fs.s3a.retry.limit", "1");
        }
        config.putIfAbsent("writeDataTimeoutMilliseconds", Math.max(1000L, timeoutMs));
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
