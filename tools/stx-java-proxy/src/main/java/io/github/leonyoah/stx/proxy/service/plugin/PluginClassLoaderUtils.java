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

import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.io.IOException;
import java.net.MalformedURLException;
import java.net.URL;
import java.net.URLClassLoader;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.stream.Stream;

public final class PluginClassLoaderUtils {

    private static final Map<String, CachedPluginClassLoader> CLASS_LOADER_CACHE =
            new ConcurrentHashMap<>();
    private static final Object CLASS_LOADER_CACHE_LOCK = new Object();

    private PluginClassLoaderUtils() {}

    /**
     * 基于显式传入的插件 jar 创建一次性 classloader，调用方用完后需要关闭。 Creates a one-shot classloader from explicit
     * plugin jars; callers must close it after use.
     */
    public static URLClassLoader createClassLoader(List<String> pluginJars, ClassLoader parent)
            throws MalformedURLException {
        List<String> normalizedJars = normalizeJarPaths(pluginJars);
        return new URLClassLoader(toURLs(normalizedJars), parent);
    }

    /**
     * 每次从 SEATUNNEL_HOME 动态扫描 connectors/plugins，并按文件指纹复用缓存 classloader。 Scans SEATUNNEL_HOME
     * connectors/plugins on every call and reuses a cached classloader by file fingerprint.
     */
    public static URLClassLoader createClassLoaderFromSeatunnelHome(ClassLoader parent)
            throws IOException {
        String seatunnelHome = resolveSeatunnelHome();
        if (StringUtils.isBlank(seatunnelHome)) {
            return null;
        }
        Path homePath = Paths.get(seatunnelHome).toAbsolutePath().normalize();
        String scope = "seatunnel_home|" + homePath + "|parent=" + System.identityHashCode(parent);
        List<String> pluginJars = collectJarPaths(homePath);
        if (pluginJars.isEmpty()) {
            retireStaleCachedClassLoaders(scope, scope + "|fingerprint=empty");
            return null;
        }
        return getOrCreateCachedClassLoader(scope, pluginJars, parent);
    }

    /**
     * 收集 SEATUNNEL_HOME 下 connectors 与 plugins 目录里的 jar。 Collects jars under the SEATUNNEL_HOME
     * connectors and plugins directories.
     */
    static List<String> collectJarPaths(Path seatunnelHome) throws IOException {
        List<String> pluginJars = new ArrayList<>();
        if (seatunnelHome == null) {
            return pluginJars;
        }
        collectJarPaths(seatunnelHome.resolve("connectors"), pluginJars);
        collectJarPaths(seatunnelHome.resolve("plugins"), pluginJars);
        collectJarPaths(seatunnelHome.resolve("lib"), pluginJars);
        collectJarPaths(seatunnelHome.resolve("starter"), pluginJars);
        collectJarPaths(seatunnelHome.resolve("seatunnel-dist"), pluginJars);

        String extraPaths = System.getProperty("stx.connector.paths");
        if (StringUtils.isBlank(extraPaths)) {
            extraPaths = System.getenv("STX_CONNECTOR_PATHS");
        }
        if (StringUtils.isNotBlank(extraPaths)) {
            for (String extraPath : extraPaths.split("[,;:]")) {
                if (StringUtils.isNotBlank(extraPath)) {
                    collectJarPaths(Paths.get(extraPath.trim()), pluginJars);
                }
            }
        }
        return pluginJars;
    }

    /**
     * 用路径、大小和修改时间生成 classpath 指纹。 Builds a classpath fingerprint from path, size, and last-modified
     * time.
     */
    static String fingerprint(List<String> paths) {
        if (paths == null || paths.isEmpty()) {
            return "empty";
        }
        List<String> parts = new ArrayList<>();
        for (String raw : normalizeJarPaths(paths)) {
            Path path = Paths.get(raw).toAbsolutePath().normalize();
            try {
                parts.add(
                        path
                                + "#"
                                + Files.size(path)
                                + "#"
                                + Files.getLastModifiedTime(path).toMillis());
            } catch (IOException e) {
                parts.add(path + "#missing");
            }
        }
        Collections.sort(parts);
        return String.join("|", parts);
    }

    /**
     * 读取 classloader 对应的 classpath 指纹。 Reads the classpath fingerprint represented by the
     * classloader.
     */
    static String classpathFingerprint(ClassLoader classLoader) {
        if (classLoader instanceof CachedPluginClassLoader) {
            return ((CachedPluginClassLoader) classLoader).getFingerprint();
        }
        if (classLoader instanceof URLClassLoader) {
            List<String> paths = new ArrayList<>();
            for (URL url : ((URLClassLoader) classLoader).getURLs()) {
                if ("file".equalsIgnoreCase(url.getProtocol())) {
                    paths.add(toPathString(url));
                }
            }
            return fingerprint(paths);
        }
        return "classpath";
    }

    /** 读取 classloader 对应的插件 jar 路径。 Reads plugin jar paths represented by the classloader. */
    static List<String> pluginJars(ClassLoader classLoader) {
        if (classLoader instanceof CachedPluginClassLoader) {
            return ((CachedPluginClassLoader) classLoader).getPluginJars();
        }
        if (classLoader instanceof URLClassLoader) {
            List<String> paths = new ArrayList<>();
            for (URL url : ((URLClassLoader) classLoader).getURLs()) {
                if ("file".equalsIgnoreCase(url.getProtocol())) {
                    paths.add(toPathString(url));
                }
            }
            return Collections.unmodifiableList(paths);
        }
        return Collections.emptyList();
    }

    /** 清空缓存供单元测试隔离状态。 Clears cached classloaders so unit tests can isolate state. */
    static void clearCachedClassLoadersForTest() {
        synchronized (CLASS_LOADER_CACHE_LOCK) {
            for (CachedPluginClassLoader classLoader : CLASS_LOADER_CACHE.values()) {
                classLoader.retire();
            }
            CLASS_LOADER_CACHE.clear();
        }
    }

    /** 从指定根目录递归收集 jar 路径。 Recursively collects jar paths from the given root directory. */
    static void collectJarPaths(Path root, List<String> pluginJars) throws IOException {
        if (root == null || !Files.exists(root)) {
            return;
        }
        try (Stream<Path> pathStream = Files.walk(root)) {
            pathStream
                    .filter(Files::isRegularFile)
                    .filter(path -> path.getFileName().toString().endsWith(".jar"))
                    .map(Path::toAbsolutePath)
                    .map(Path::toString)
                    .sorted()
                    .forEach(pluginJars::add);
        }
    }

    /**
     * 释放 classloader；缓存 classloader 会转为释放租约，过期后再真正关闭。 Releases a classloader; cached classloaders
     * release their lease and close only after expiry.
     */
    public static void closeQuietly(ClassLoader classLoader) {
        if (classLoader instanceof URLClassLoader) {
            try {
                ((URLClassLoader) classLoader).close();
            } catch (IOException ignored) {
                // no-op
            }
        }
    }

    /**
     * 解析当前 proxy 绑定的 SeaTunnel 安装目录。 Resolves the SeaTunnel installation directory used by the
     * current proxy.
     */
    private static String resolveSeatunnelHome() {
        String seatunnelHome = System.getProperty("stx.java.proxy.seatunnel.home");
        if (StringUtils.isBlank(seatunnelHome)) {
            seatunnelHome = System.getProperty("SEATUNNEL_HOME");
        }
        if (StringUtils.isBlank(seatunnelHome)) {
            seatunnelHome = System.getProperty("seatunnel.home");
        }
        if (StringUtils.isBlank(seatunnelHome)) {
            seatunnelHome = System.getenv("SEATUNNEL_HOME");
        }
        if (StringUtils.isBlank(seatunnelHome)) {
            seatunnelHome = System.getenv("STX_JAVA_PROXY_SEATUNNEL_HOME");
        }
        return seatunnelHome;
    }

    /**
     * 按 scope 与文件指纹获取或创建缓存 classloader。 Gets or creates a cached classloader by scope and file
     * fingerprint.
     */
    private static CachedPluginClassLoader getOrCreateCachedClassLoader(
            String scope, List<String> pluginJars, ClassLoader parent) throws IOException {
        List<String> normalizedJars = normalizeJarPaths(pluginJars);
        String fingerprint = fingerprint(normalizedJars);
        String cacheKey = scope + "|fingerprint=" + fingerprint;
        CachedPluginClassLoader cached = CLASS_LOADER_CACHE.get(cacheKey);
        if (cached != null) {
            return cached.acquire();
        }
        synchronized (CLASS_LOADER_CACHE_LOCK) {
            cached = CLASS_LOADER_CACHE.get(cacheKey);
            if (cached != null) {
                return cached.acquire();
            }
            retireStaleCachedClassLoaders(scope, cacheKey);
            CachedPluginClassLoader created =
                    new CachedPluginClassLoader(
                            cacheKey, fingerprint, normalizedJars, toURLs(normalizedJars), parent);
            CLASS_LOADER_CACHE.put(cacheKey, created);
            return created.acquire();
        }
    }

    /**
     * 退休同一 scope 下已不匹配当前指纹的旧 classloader。 Retires stale classloaders under the same scope whose
     * fingerprint no longer matches.
     */
    private static void retireStaleCachedClassLoaders(String scope, String activeCacheKey) {
        String prefix = scope + "|";
        for (Map.Entry<String, CachedPluginClassLoader> entry : CLASS_LOADER_CACHE.entrySet()) {
            if (!entry.getKey().startsWith(prefix) || entry.getKey().equals(activeCacheKey)) {
                continue;
            }
            CachedPluginClassLoader stale = CLASS_LOADER_CACHE.remove(entry.getKey());
            if (stale != null) {
                stale.retire();
            }
        }
    }

    /** 规范化、去重并排序插件 jar 路径。 Normalizes, deduplicates, and sorts plugin jar paths. */
    private static List<String> normalizeJarPaths(List<String> pluginJars) {
        if (pluginJars == null || pluginJars.isEmpty()) {
            return Collections.emptyList();
        }
        LinkedHashSet<String> normalized = new LinkedHashSet<>();
        for (String raw : pluginJars) {
            if (StringUtils.isBlank(raw)) {
                continue;
            }
            Path jarPath = Paths.get(raw).toAbsolutePath().normalize();
            if (!Files.exists(jarPath)) {
                throw new ProxyException(400, "Plugin jar does not exist: " + jarPath);
            }
            normalized.add(jarPath.toString());
        }
        List<String> result = new ArrayList<>(normalized);
        Collections.sort(result);
        return Collections.unmodifiableList(result);
    }

    /** 将 jar 路径转换为 URLClassLoader 可用的 URL 数组。 Converts jar paths into URLs for URLClassLoader. */
    private static URL[] toURLs(List<String> pluginJars) throws MalformedURLException {
        URL[] urls = new URL[pluginJars.size()];
        for (int i = 0; i < pluginJars.size(); i++) {
            urls[i] = Paths.get(pluginJars.get(i)).toUri().toURL();
        }
        return urls;
    }

    /** 将 file URL 安全转换为本地路径字符串。 Safely converts a file URL into a local path string. */
    private static String toPathString(URL url) {
        try {
            return Paths.get(url.toURI()).toAbsolutePath().normalize().toString();
        } catch (Exception e) {
            return Paths.get(url.getPath()).toAbsolutePath().normalize().toString();
        }
    }

    static final class CachedPluginClassLoader extends URLClassLoader {
        private final String cacheKey;
        private final String fingerprint;
        private final List<String> pluginJars;
        private final AtomicInteger leases = new AtomicInteger();
        private volatile boolean retired;
        private volatile boolean closed;

        /** 创建带租约计数的缓存 classloader。 Creates a cached classloader with lease counting. */
        CachedPluginClassLoader(
                String cacheKey,
                String fingerprint,
                List<String> pluginJars,
                URL[] urls,
                ClassLoader parent) {
            super(urls, parent);
            this.cacheKey = cacheKey;
            this.fingerprint = fingerprint;
            this.pluginJars = Collections.unmodifiableList(new ArrayList<>(pluginJars));
        }

        /** 为一次请求获取 classloader 租约。 Acquires one request lease for this classloader. */
        CachedPluginClassLoader acquire() {
            leases.incrementAndGet();
            return this;
        }

        /**
         * 返回当前 classloader 的 classpath 指纹。 Returns the classpath fingerprint of this classloader.
         */
        String getFingerprint() {
            return fingerprint;
        }

        /** 返回当前 classloader 管理的插件 jar。 Returns plugin jars managed by this classloader. */
        List<String> getPluginJars() {
            return pluginJars;
        }

        /**
         * 请求结束时释放租约；只有退休且无租约时才真正关闭。 Releases a lease at request end; it closes only when retired
         * and no leases remain.
         */
        @Override
        public void close() {
            int remaining = leases.updateAndGet(current -> current <= 0 ? 0 : current - 1);
            if (retired && remaining == 0) {
                closeActualQuietly();
            }
        }

        /**
         * 标记缓存过期，并在无请求使用时关闭底层 URLClassLoader。 Marks the cache entry retired and closes the
         * underlying URLClassLoader when idle.
         */
        void retire() {
            retired = true;
            if (leases.get() == 0) {
                closeActualQuietly();
            }
        }

        /** 幂等关闭底层 URLClassLoader。 Idempotently closes the underlying URLClassLoader. */
        private synchronized void closeActualQuietly() {
            if (closed) {
                return;
            }
            try {
                super.close();
            } catch (IOException ignored) {
                // no-op
            }
            closed = true;
        }

        /** 返回便于诊断的缓存状态。 Returns cache state for diagnostics. */
        @Override
        public String toString() {
            return "CachedPluginClassLoader{"
                    + "cacheKey='"
                    + cacheKey
                    + '\''
                    + ", leases="
                    + leases.get()
                    + ", retired="
                    + retired
                    + '}';
        }
    }
}
