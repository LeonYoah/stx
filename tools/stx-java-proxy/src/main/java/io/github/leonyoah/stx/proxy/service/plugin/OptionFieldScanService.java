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

import org.apache.seatunnel.api.configuration.Option;
import org.apache.seatunnel.api.table.factory.Factory;

import io.github.leonyoah.stx.proxy.model.OptionOrigin;
import io.github.leonyoah.stx.proxy.model.PluginOptionDescriptor;
import io.github.leonyoah.stx.proxy.model.RequiredMode;

import java.io.IOException;
import java.lang.reflect.Field;
import java.lang.reflect.Modifier;
import java.net.URL;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Enumeration;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.jar.JarEntry;
import java.util.jar.JarFile;
import java.util.stream.Stream;

class OptionFieldScanService {

    private static final String PACKAGE_PREFIX = "org/apache/seatunnel/";
    private static final String[] CLASS_SUFFIXES = {
        "Options.class",
        "BaseOptions.class",
        "CommonOptions.class",
        "SourceOptions.class",
        "SinkOptions.class",
        "CatalogOptions.class",
        "ConfigOptions.class"
    };
    private static final Map<String, ScanResult> FIELD_SCAN_CACHE = new ConcurrentHashMap<>();

    ScanResult scan(
            Factory factory,
            PluginRuntimeService.PluginExecutionContext context,
            PluginRuntimeService runtimeService) {
        List<String> locations = new ArrayList<>();
        if (!context.getPluginJars().isEmpty()) {
            locations.addAll(context.getPluginJars());
        }
        String factoryLocation = runtimeService.resolveCodeSourceLocation(factory);
        if (!locations.contains(factoryLocation)) {
            locations.add(factoryLocation);
        }
        String fingerprint =
                context.getClasspathFingerprint()
                        + "|"
                        + runtimeService.resolveCodeSourceFingerprint(factory);
        ScanResult cached = FIELD_SCAN_CACHE.get(fingerprint);
        if (cached != null) {
            return cached;
        }
        Map<String, PluginOptionDescriptor> descriptors = new LinkedHashMap<>();
        List<String> warnings = new ArrayList<>();
        for (String location : locations) {
            try {
                scanLocation(location, context.getClassLoader(), descriptors);
            } catch (Exception e) {
                warnings.add("Option field scan skipped for " + location + ": " + e.getMessage());
            }
        }
        ScanResult result = new ScanResult(new ArrayList<>(descriptors.values()), warnings);
        FIELD_SCAN_CACHE.put(fingerprint, result);
        return result;
    }

    private void scanLocation(
            String location,
            ClassLoader classLoader,
            Map<String, PluginOptionDescriptor> descriptors)
            throws Exception {
        if (location == null) {
            return;
        }
        Path path = resolvePath(location);
        if (path == null || !Files.exists(path)) {
            return;
        }
        if (Files.isDirectory(path)) {
            scanDirectory(path, classLoader, descriptors);
            return;
        }
        if (location.endsWith(".jar")) {
            scanJar(path, classLoader, descriptors);
        }
    }

    private void scanDirectory(
            Path root, ClassLoader classLoader, Map<String, PluginOptionDescriptor> descriptors)
            throws IOException {
        try (Stream<Path> stream = Files.walk(root)) {
            stream.filter(Files::isRegularFile)
                    .filter(path -> matchesCandidate(path.toString().replace('\\', '/')))
                    .forEach(
                            path ->
                                    inspectClass(
                                            toClassName(root, path), classLoader, descriptors));
        }
    }

    private void scanJar(
            Path jarPath, ClassLoader classLoader, Map<String, PluginOptionDescriptor> descriptors)
            throws IOException {
        try (JarFile jarFile = new JarFile(jarPath.toFile())) {
            Enumeration<JarEntry> entries = jarFile.entries();
            while (entries.hasMoreElements()) {
                JarEntry entry = entries.nextElement();
                if (entry.isDirectory() || !matchesCandidate(entry.getName())) {
                    continue;
                }
                inspectClass(
                        entry.getName().replace('/', '.').replace(".class", ""),
                        classLoader,
                        descriptors);
            }
        }
    }

    private void inspectClass(
            String className,
            ClassLoader classLoader,
            Map<String, PluginOptionDescriptor> descriptors) {
        try {
            Class<?> clazz = Class.forName(className, false, classLoader);
            Set<Class<?>> hierarchy = new LinkedHashSet<>();
            for (Class<?> current = clazz;
                    current != null && !Object.class.equals(current);
                    current = current.getSuperclass()) {
                hierarchy.add(current);
            }
            for (Class<?> current : hierarchy) {
                for (Field field : current.getDeclaredFields()) {
                    if (!Modifier.isStatic(field.getModifiers())
                            || !Option.class.isAssignableFrom(field.getType())) {
                        continue;
                    }
                    field.setAccessible(true);
                    Object value = field.get(null);
                    if (!(value instanceof Option)) {
                        continue;
                    }
                    Option<?> option = (Option<?>) value;
                    PluginOptionDescriptor descriptor =
                            PluginOptionSupport.buildDescriptor(
                                    option,
                                    option.defaultValue() == null
                                            ? RequiredMode.UNKNOWN_NO_DEFAULT
                                            : RequiredMode.SUPPLEMENTAL_OPTIONAL,
                                    null,
                                    null,
                                    OptionOrigin.FIELD_SCAN,
                                    current.getName(),
                                    true);
                    PluginOptionDescriptor existing = descriptors.get(option.key());
                    if (existing == null) {
                        descriptors.put(option.key(), descriptor);
                    } else {
                        PluginOptionSupport.merge(existing, descriptor);
                    }
                }
            }
        } catch (Throwable ignored) {
            // Ignore classes that cannot be loaded or reflected safely.
            // 忽略无法安全加载或反射的类。
        }
    }

    private boolean matchesCandidate(String entryName) {
        if (!entryName.startsWith(PACKAGE_PREFIX)) {
            return false;
        }
        for (String suffix : CLASS_SUFFIXES) {
            if (entryName.endsWith(suffix)) {
                return true;
            }
        }
        return false;
    }

    private String toClassName(Path root, Path file) {
        String relative = root.relativize(file).toString().replace('\\', '/');
        return relative.replace('/', '.').replace(".class", "");
    }

    private Path resolvePath(String location) {
        try {
            if (location.startsWith("file:")) {
                return Paths.get(new URL(location).toURI());
            }
            return Paths.get(location);
        } catch (Exception e) {
            return null;
        }
    }

    static class ScanResult {
        private final List<PluginOptionDescriptor> options;
        private final List<String> warnings;

        ScanResult(List<PluginOptionDescriptor> options, List<String> warnings) {
            this.options = Collections.unmodifiableList(options);
            this.warnings = Collections.unmodifiableList(warnings);
        }

        List<PluginOptionDescriptor> getOptions() {
            return options;
        }

        List<String> getWarnings() {
            return warnings;
        }
    }
}
