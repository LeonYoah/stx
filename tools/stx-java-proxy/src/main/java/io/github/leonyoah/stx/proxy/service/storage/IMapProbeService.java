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

import org.apache.seatunnel.engine.common.utils.FactoryUtil;
import org.apache.seatunnel.engine.imap.storage.api.IMapStorage;
import org.apache.seatunnel.engine.imap.storage.api.IMapStorageFactory;

import io.github.leonyoah.stx.proxy.service.plugin.PluginClassLoaderUtils;
import io.github.leonyoah.stx.proxy.service.support.ProbeExecutionUtils;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.net.URLClassLoader;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.UUID;

public class IMapProbeService {

    public Map<String, Object> probe(Map<String, Object> request) {
        return ProbeExecutionUtils.runWithTimeout(
                request,
                "IMap probe",
                StorageProbeConfigSupport.imapTimeoutHint(),
                () -> doProbe(request));
    }

    private Map<String, Object> doProbe(Map<String, Object> request) {
        String plugin = ProxyRequestUtils.getOptionalString(request, "plugin");
        if (plugin == null) {
            plugin = "hdfs";
        }
        if (!"hdfs".equalsIgnoreCase(plugin)) {
            throw new ProxyException(400, "Unsupported imap plugin: " + plugin + ", expected hdfs");
        }
        String mode = ProxyRequestUtils.getOptionalString(request, "mode");
        if (mode == null) {
            mode = "read_write";
        }
        List<String> pluginJars = ProxyRequestUtils.getStringList(request, "pluginJars");
        boolean deleteAllOnDestroy =
                ProxyRequestUtils.getBoolean(request, "deleteAllOnDestroy", false);
        Map<String, Object> config = ProxyRequestUtils.getMap(request, "config");
        validateStorageType(ProxyRequestUtils.getOptionalString(config, "storage.type"));
        StorageProbeConfigSupport.validateIMapConfig(config);
        StorageProbeConfigSupport.applyIMapFastFailDefaults(
                config, ProbeExecutionUtils.resolveProbeTimeoutMs(request));

        StorageHandle<IMapStorage> storageHandle = createStorage(plugin, pluginJars, config);
        IMapStorage storage = storageHandle.getStorage();
        Map<String, Object> response = new LinkedHashMap<>();
        response.put("ok", true);
        response.put("plugin", plugin);
        response.put("mode", mode);
        response.put("storageType", ProxyRequestUtils.getOptionalString(config, "storage.type"));

        if ("init_only".equalsIgnoreCase(mode)) {
            response.put("initialized", true);
            try {
                storage.destroy(deleteAllOnDestroy);
            } finally {
                storageHandle.close();
            }
            return response;
        }
        if (!"read_write".equalsIgnoreCase(mode)) {
            try {
                storage.destroy(deleteAllOnDestroy);
            } finally {
                storageHandle.close();
            }
            throw new ProxyException(400, "Unsupported imap probe mode: " + mode);
        }

        String key = "stx-java-proxy-key-" + UUID.randomUUID();
        String value = "stx-java-proxy-value";
        boolean writable = false;
        boolean readable = false;

        try {
            writable = storage.store(key, value);
            Set<Object> keys = storage.loadAllKeys();
            readable = keys != null && keys.contains(key);
            storage.delete(key);
        } catch (Exception e) {
            throw new ProxyException(500, "IMap probe failed: " + e.getMessage(), e);
        } finally {
            try {
                storage.destroy(deleteAllOnDestroy);
            } catch (Exception ignored) {
                // ignore destroy exceptions
            }
            storageHandle.close();
        }

        response.put("writable", writable);
        response.put("readable", readable);
        return response;
    }

    private void validateStorageType(String storageType) {
        if (storageType == null) {
            throw new ProxyException(400, "IMap probe requires remote config field: storage.type");
        }
        if ("local".equalsIgnoreCase(storageType) || "localfile".equalsIgnoreCase(storageType)) {
            throw new ProxyException(
                    400,
                    "Local imap storage is not supported, use remote storage.type such as hdfs, s3, or oss");
        }
    }

    private StorageHandle<IMapStorage> createStorage(
            String plugin, List<String> pluginJars, Map<String, Object> config) {
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
                IMapStorage storage =
                        FactoryUtil.discoverFactory(
                                        probeClassLoader, IMapStorageFactory.class, plugin)
                                .create(config);
                return new StorageHandle<>(storage, urlClassLoader);
            } finally {
                currentThread.setContextClassLoader(originalClassLoader);
            }
        } catch (UnsupportedOperationException e) {
            PluginClassLoaderUtils.closeQuietly(urlClassLoader);
            throw new ProxyException(
                    500,
                    "IMap probe failed during initialization: "
                            + e.getMessage()
                            + ". This usually indicates the current JDK/Hadoop combination cannot use Subject.getSubject.",
                    e);
        } catch (Exception e) {
            PluginClassLoaderUtils.closeQuietly(urlClassLoader);
            throw new ProxyException(
                    500, "IMap probe failed during initialization: " + e.getMessage(), e);
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
