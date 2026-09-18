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

import org.apache.seatunnel.api.table.factory.TableSourceFactory;

import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import io.github.leonyoah.stx.proxy.model.PluginFactoryInfo;

import java.io.IOException;
import java.net.URLClassLoader;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;
import java.util.jar.JarEntry;
import java.util.jar.JarOutputStream;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotSame;
import static org.junit.jupiter.api.Assertions.assertSame;

class PluginClassLoaderUtilsTest {

    @TempDir Path tempDir;

    @Test
    void collectJarPathsShouldIncludeConnectorsAndPlugins() throws IOException {
        Path connectorsJar = tempDir.resolve("connectors/mysql/mysql-connector.jar");
        Path pluginJar = tempDir.resolve("plugins/Jdbc/lib/postgresql-driver.jar");
        Path ignoredFile = tempDir.resolve("plugins/Jdbc/lib/README.txt");

        Files.createDirectories(connectorsJar.getParent());
        Files.createDirectories(pluginJar.getParent());
        Files.write(connectorsJar, new byte[0]);
        Files.write(pluginJar, new byte[0]);
        Files.write(ignoredFile, new byte[0]);

        List<String> pluginJars = PluginClassLoaderUtils.collectJarPaths(tempDir);

        assertEquals(
                Arrays.asList(
                        connectorsJar.toAbsolutePath().toString(),
                        pluginJar.toAbsolutePath().toString()),
                pluginJars);
    }

    @Test
    void createClassLoaderFromSeatunnelHomeShouldReuseCacheUntilJarSetChanges() throws Exception {
        PluginClassLoaderUtils.clearCachedClassLoadersForTest();
        String previousHome = System.getProperty("SEATUNNEL_HOME");
        try {
            System.setProperty("SEATUNNEL_HOME", tempDir.toString());
            Path connectorsJar = tempDir.resolve("connectors/mysql/mysql-connector.jar");
            Files.createDirectories(connectorsJar.getParent());
            Files.write(connectorsJar, new byte[] {1});

            ClassLoader parent = Thread.currentThread().getContextClassLoader();
            ClassLoader first = PluginClassLoaderUtils.createClassLoaderFromSeatunnelHome(parent);
            ClassLoader second = PluginClassLoaderUtils.createClassLoaderFromSeatunnelHome(parent);

            assertSame(first, second);
            assertEquals(
                    PluginClassLoaderUtils.classpathFingerprint(first),
                    PluginClassLoaderUtils.classpathFingerprint(second));

            PluginClassLoaderUtils.closeQuietly(first);
            PluginClassLoaderUtils.closeQuietly(second);

            Path pluginJar = tempDir.resolve("plugins/Jdbc/lib/postgresql-driver.jar");
            Files.createDirectories(pluginJar.getParent());
            Files.write(pluginJar, new byte[] {2});

            ClassLoader changed = PluginClassLoaderUtils.createClassLoaderFromSeatunnelHome(parent);
            assertNotSame(first, changed);
            assertEquals(2, PluginClassLoaderUtils.pluginJars(changed).size());
            PluginClassLoaderUtils.closeQuietly(changed);
        } finally {
            if (previousHome == null) {
                System.clearProperty("SEATUNNEL_HOME");
            } else {
                System.setProperty("SEATUNNEL_HOME", previousHome);
            }
            PluginClassLoaderUtils.clearCachedClassLoadersForTest();
        }
    }

    @Test
    void discoverPluginFactoriesShouldSkipProviderWithMissingDependency() throws Exception {
        Path brokenJar = tempDir.resolve("connectors/cdc/connector-cdc-mysql.jar");
        Files.createDirectories(brokenJar.getParent());
        try (JarOutputStream jarOutputStream =
                new JarOutputStream(Files.newOutputStream(brokenJar))) {
            jarOutputStream.putNextEntry(
                    new JarEntry("META-INF/services/" + TableSourceFactory.class.getName()));
            jarOutputStream.write(
                    "org.apache.seatunnel.connectors.cdc.mysql.source.MySqlTableSourceFactory\n"
                            .getBytes(java.nio.charset.StandardCharsets.UTF_8));
            jarOutputStream.closeEntry();
        }

        URLClassLoader classLoader =
                PluginClassLoaderUtils.createClassLoader(
                        Collections.singletonList(brokenJar.toString()),
                        Thread.currentThread().getContextClassLoader());
        PluginRuntimeService.PluginExecutionContext context =
                new PluginRuntimeService.PluginExecutionContext(
                        "source",
                        PluginClassLoaderUtils.pluginJars(classLoader),
                        classLoader,
                        classLoader,
                        PluginClassLoaderUtils.classpathFingerprint(classLoader),
                        "seatunnel_home",
                        new ArrayList<>());
        try {
            List<PluginFactoryInfo> factories =
                    new PluginRuntimeService().discoverPluginFactories(context);

            assertEquals(0, factories.size());
            assertEquals(1, context.getWarnings().size());
        } finally {
            context.close();
        }
    }
}
