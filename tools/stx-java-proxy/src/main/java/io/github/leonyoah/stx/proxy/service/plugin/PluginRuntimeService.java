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

package io.github.leonyoah.stx.proxy.service.plugin;

import org.apache.seatunnel.shade.org.apache.commons.lang3.StringUtils;

import org.apache.seatunnel.api.table.factory.CatalogFactory;
import org.apache.seatunnel.api.table.factory.Factory;
import org.apache.seatunnel.api.table.factory.FactoryUtil;
import org.apache.seatunnel.api.table.factory.TableSinkFactory;
import org.apache.seatunnel.api.table.factory.TableSourceFactory;
import org.apache.seatunnel.api.table.factory.TableTransformFactory;

import io.github.leonyoah.stx.proxy.model.PluginFactoryInfo;
import io.github.leonyoah.stx.proxy.model.PluginListResult;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.URL;
import java.net.URLClassLoader;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Comparator;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.jar.JarEntry;
import java.util.jar.JarFile;

public class PluginRuntimeService {

    private static final Map<String, List<PluginFactoryInfo>> LIST_CACHE =
            new ConcurrentHashMap<>();

    public PluginListResult list(Map<String, Object> request) {
        PluginExecutionContext context = openContext(request);
        Thread currentThread = Thread.currentThread();
        ClassLoader originalClassLoader = currentThread.getContextClassLoader();
        try {
            currentThread.setContextClassLoader(context.getClassLoader());
            String cacheKey = context.getPluginType() + "|" + context.getClasspathFingerprint();
            List<PluginFactoryInfo> plugins = LIST_CACHE.get(cacheKey);
            if (plugins == null) {
                plugins = discoverPluginFactories(context);
                LIST_CACHE.put(cacheKey, plugins);
            }
            return new PluginListResult(
                    true, context.getPluginType(), plugins, context.getWarnings());
        } finally {
            currentThread.setContextClassLoader(originalClassLoader);
            context.close();
        }
    }

    PluginExecutionContext openContext(Map<String, Object> request) {
        String pluginType =
                normalizePluginType(ProxyRequestUtils.getRequiredString(request, "pluginType"));
        List<String> pluginJars = ProxyRequestUtils.getStringList(request, "pluginJars");
        ClassLoader parent = Thread.currentThread().getContextClassLoader();
        URLClassLoader urlClassLoader = null;
        String fingerprint = "classpath";
        List<String> warnings = new ArrayList<>();
        String origin = "classpath";
        try {
            if (!pluginJars.isEmpty()) {
                urlClassLoader = PluginClassLoaderUtils.createClassLoader(pluginJars, parent);
                fingerprint = PluginClassLoaderUtils.fingerprint(pluginJars);
                origin = "request_plugin_jars";
            } else {
                urlClassLoader = PluginClassLoaderUtils.createClassLoaderFromSeatunnelHome(parent);
                if (urlClassLoader != null) {
                    fingerprint = PluginClassLoaderUtils.classpathFingerprint(urlClassLoader);
                    pluginJars = PluginClassLoaderUtils.pluginJars(urlClassLoader);
                    origin = "seatunnel_home";
                }
            }
        } catch (IOException e) {
            throw new ProxyException(
                    500, "Failed to prepare plugin runtime classloader: " + e.getMessage(), e);
        }
        return new PluginExecutionContext(
                pluginType,
                pluginJars,
                urlClassLoader == null ? parent : urlClassLoader,
                urlClassLoader,
                fingerprint,
                origin,
                warnings);
    }

    Factory discoverFactory(PluginExecutionContext context, String factoryIdentifier) {
        try {
            switch (context.getPluginType()) {
                case "source":
                    return FactoryUtil.discoverFactory(
                            context.getClassLoader(), TableSourceFactory.class, factoryIdentifier);
                case "sink":
                    return FactoryUtil.discoverFactory(
                            context.getClassLoader(), TableSinkFactory.class, factoryIdentifier);
                case "transform":
                    return FactoryUtil.discoverFactory(
                            context.getClassLoader(),
                            TableTransformFactory.class,
                            factoryIdentifier);
                case "catalog":
                    return FactoryUtil.discoverFactory(
                            context.getClassLoader(), CatalogFactory.class, factoryIdentifier);
                default:
                    throw new ProxyException(
                            400, "Unsupported plugin type: " + context.getPluginType());
            }
        } catch (Throwable e) {
            throw new ProxyException(
                    400,
                    "Plugin factory not found for type="
                            + context.getPluginType()
                            + ", identifier="
                            + factoryIdentifier
                            + ": "
                            + e.getMessage(),
                    e);
        }
    }

    List<PluginFactoryInfo> discoverPluginFactories(PluginExecutionContext context) {
        List<? extends Factory> factories;
        switch (context.getPluginType()) {
            case "source":
                factories = discoverFactories(context, TableSourceFactory.class);
                break;
            case "sink":
                factories = discoverFactories(context, TableSinkFactory.class);
                break;
            case "transform":
                factories = discoverFactories(context, TableTransformFactory.class);
                break;
            case "catalog":
                factories = discoverFactories(context, CatalogFactory.class);
                break;
            default:
                throw new ProxyException(
                        400, "Unsupported plugin type: " + context.getPluginType());
        }
        Map<String, PluginFactoryInfo> deduped = new LinkedHashMap<>();
        for (Factory factory : factories) {
            String identifier = StringUtils.trimToEmpty(factory.factoryIdentifier());
            if (StringUtils.isBlank(identifier)) {
                continue;
            }
            deduped.put(
                    identifier.toLowerCase(),
                    new PluginFactoryInfo(
                            identifier, factory.getClass().getName(), context.origin));
        }
        List<PluginFactoryInfo> result = new ArrayList<>(deduped.values());
        result.sort(
                Comparator.comparing(
                        PluginFactoryInfo::getFactoryIdentifier, String.CASE_INSENSITIVE_ORDER));
        return Collections.unmodifiableList(result);
    }

    /**
     * 发现插件工厂时隔离坏 provider，避免一个缺依赖的 connector 打断整个列表请求。 Discovers plugin factories while isolating
     * bad providers so one connector with missing dependencies cannot break the whole list request.
     */
    private <T extends Factory> List<T> discoverFactories(
            PluginExecutionContext context, Class<T> factoryClass) {
        if (context.getPluginJars().isEmpty()) {
            try {
                return FactoryUtil.discoverFactories(context.getClassLoader(), factoryClass);
            } catch (Throwable e) {
                throw new ProxyException(
                        500, "Failed to discover plugin factories: " + summarizeThrowable(e), e);
            }
        }

        List<T> factories = new ArrayList<>();
        Set<String> providerClasses = collectProviderClasses(context, factoryClass);
        for (String providerClass : providerClasses) {
            try {
                Class<?> rawClass = Class.forName(providerClass, true, context.getClassLoader());
                if (!factoryClass.isAssignableFrom(rawClass)) {
                    continue;
                }
                factories.add(factoryClass.cast(rawClass.getDeclaredConstructor().newInstance()));
            } catch (Throwable e) {
                context.addWarning(
                        "Skip plugin provider "
                                + providerClass
                                + " because it failed to load: "
                                + summarizeThrowable(e));
            }
        }
        return factories;
    }

    /**
     * 从 jar 的 META-INF/services 文件收集 provider 类名。 Collects provider class names from jar
     * META-INF/services files.
     */
    private <T extends Factory> Set<String> collectProviderClasses(
            PluginExecutionContext context, Class<T> factoryClass) {
        Set<String> providerClasses = new LinkedHashSet<>();
        List<String> serviceEntryNames = new ArrayList<>();
        serviceEntryNames.add("META-INF/services/" + factoryClass.getName());
        if (!Factory.class.equals(factoryClass)) {
            serviceEntryNames.add("META-INF/services/" + Factory.class.getName());
        }

        for (String pluginJar : context.getPluginJars()) {
            try (JarFile jarFile = new JarFile(pluginJar)) {
                for (String serviceEntryName : serviceEntryNames) {
                    JarEntry serviceEntry = jarFile.getJarEntry(serviceEntryName);
                    if (serviceEntry == null) {
                        continue;
                    }
                    String content =
                            new String(
                                    readAllBytes(jarFile.getInputStream(serviceEntry)),
                                    StandardCharsets.UTF_8);
                    for (String line : content.split("\\R")) {
                        String providerClass = stripServiceComment(line);
                        if (StringUtils.isNotBlank(providerClass)) {
                            providerClasses.add(providerClass);
                        }
                    }
                }
            } catch (Throwable e) {
                context.addWarning(
                        "Skip service file in "
                                + pluginJar
                                + " because it failed to read: "
                                + summarizeThrowable(e));
            }
        }

        for (String serviceEntryName : serviceEntryNames) {
            try {
                java.util.Enumeration<URL> resources =
                        context.getClassLoader().getResources(serviceEntryName);
                while (resources.hasMoreElements()) {
                    URL resourceUrl = resources.nextElement();
                    try (InputStream in = resourceUrl.openStream()) {
                        String content = new String(readAllBytes(in), StandardCharsets.UTF_8);
                        for (String line : content.split("\\R")) {
                            String providerClass = stripServiceComment(line);
                            if (StringUtils.isNotBlank(providerClass)) {
                                providerClasses.add(providerClass);
                            }
                        }
                    } catch (Throwable ignored) {
                    }
                }
            } catch (Throwable ignored) {
            }
        }
        return providerClasses;
    }

    /**
     * 读取 service 文件内容，避免依赖较新 JDK 的 InputStream.readAllBytes。 Reads service file content without
     * relying on newer JDK InputStream.readAllBytes.
     */
    private byte[] readAllBytes(InputStream inputStream) throws IOException {
        byte[] buffer = new byte[4096];
        int bytesRead;
        try (InputStream in = inputStream;
                ByteArrayOutputStream out = new ByteArrayOutputStream()) {
            while ((bytesRead = in.read(buffer)) >= 0) {
                out.write(buffer, 0, bytesRead);
            }
            return out.toByteArray();
        }
    }

    /**
     * 去掉 ServiceLoader 配置行中的注释和空白。 Removes comments and whitespace from one ServiceLoader
     * configuration line.
     */
    private String stripServiceComment(String line) {
        String value = StringUtils.trimToEmpty(line);
        int commentIndex = value.indexOf('#');
        if (commentIndex >= 0) {
            value = value.substring(0, commentIndex);
        }
        return StringUtils.trimToEmpty(value);
    }

    /** 压缩异常链，便于作为 API warning 返回。 Summarizes the throwable chain for API warnings. */
    private String summarizeThrowable(Throwable throwable) {
        if (throwable == null) {
            return "unknown";
        }
        List<String> parts = new ArrayList<>();
        Throwable current = throwable;
        while (current != null && parts.size() < 4) {
            String simpleName = current.getClass().getSimpleName();
            String message = StringUtils.trimToEmpty(current.getMessage());
            parts.add(StringUtils.isBlank(message) ? simpleName : simpleName + ": " + message);
            current = current.getCause();
        }
        return String.join(" | caused by: ", parts);
    }

    String resolveCodeSourceFingerprint(Factory factory) {
        try {
            URL url = FactoryUtil.getFactoryUrl(factory);
            if (url == null || !"file".equalsIgnoreCase(url.getProtocol())) {
                return String.valueOf(url);
            }
            return PluginClassLoaderUtils.fingerprint(
                    Collections.singletonList(java.nio.file.Paths.get(url.toURI()).toString()));
        } catch (Exception e) {
            return factory.getClass().getName();
        }
    }

    String resolveCodeSourceLocation(Factory factory) {
        try {
            URL url = FactoryUtil.getFactoryUrl(factory);
            return url == null ? factory.getClass().getName() : String.valueOf(url);
        } catch (Exception e) {
            return factory.getClass().getName();
        }
    }

    private String normalizePluginType(String pluginType) {
        String normalized = StringUtils.trimToEmpty(pluginType).toLowerCase();
        switch (normalized) {
            case "source":
            case "sink":
            case "transform":
            case "catalog":
                return normalized;
            default:
                throw new ProxyException(400, "Unsupported pluginType: " + pluginType);
        }
    }

    static class PluginExecutionContext implements AutoCloseable {
        private final String pluginType;
        private final List<String> pluginJars;
        private final ClassLoader classLoader;
        private final URLClassLoader closeableClassLoader;
        private final String classpathFingerprint;
        private final String origin;
        private final List<String> warnings;

        PluginExecutionContext(
                String pluginType,
                List<String> pluginJars,
                ClassLoader classLoader,
                URLClassLoader closeableClassLoader,
                String classpathFingerprint,
                String origin,
                List<String> warnings) {
            this.pluginType = pluginType;
            this.pluginJars = pluginJars;
            this.classLoader = classLoader;
            this.closeableClassLoader = closeableClassLoader;
            this.classpathFingerprint = classpathFingerprint;
            this.origin = origin;
            this.warnings = warnings;
        }

        String getPluginType() {
            return pluginType;
        }

        List<String> getPluginJars() {
            return pluginJars;
        }

        ClassLoader getClassLoader() {
            return classLoader;
        }

        String getClasspathFingerprint() {
            return classpathFingerprint;
        }

        List<String> getWarnings() {
            return warnings;
        }

        void addWarning(String warning) {
            if (StringUtils.isNotBlank(warning)) {
                warnings.add(warning);
            }
        }

        @Override
        public void close() {
            PluginClassLoaderUtils.closeQuietly(closeableClassLoader);
        }
    }
}
