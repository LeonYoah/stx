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
import org.apache.hadoop.fs.Path;

import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.LinkedHashMap;
import java.util.Map;

public class StorageProbeConfigSupportTest {

    @TempDir java.nio.file.Path tempDir;

    @Test
    public void testBuildHadoopConfigurationLoadsHdfsSitePathAndCustomHaNamenodes()
            throws Exception {
        java.nio.file.Path siteXml = tempDir.resolve("hdfs-site.xml");
        String xml =
                "<?xml version=\"1.0\"?>\n"
                        + "<configuration>\n"
                        + "  <property><name>dfs.nameservices</name><value>stx-ha</value></property>\n"
                        + "  <property><name>dfs.ha.namenodes.stx-ha</name><value>nn-alpha,nn-beta</value></property>\n"
                        + "  <property><name>dfs.namenode.rpc-address.stx-ha.nn-alpha</name><value>127.0.0.1:18020</value></property>\n"
                        + "  <property><name>dfs.namenode.rpc-address.stx-ha.nn-beta</name><value>127.0.0.1:28020</value></property>\n"
                        + "  <property><name>dfs.client.failover.proxy.provider.stx-ha</name><value>org.apache.hadoop.hdfs.server.namenode.ha.ConfiguredFailoverProxyProvider</value></property>\n"
                        + "</configuration>\n";
        Files.write(siteXml, xml.getBytes(StandardCharsets.UTF_8));

        Map<String, String> config = new LinkedHashMap<>();
        config.put("storage.type", "hdfs");
        config.put("namespace", "/seatunnel/checkpoint/");
        config.put("hdfs_site_path", siteXml.toString());
        config.put("disable.cache", "true");
        config.put("seatunnel.hadoop.dfs.client.socket-timeout", "5000");

        Configuration conf = StorageProbeConfigSupport.buildHadoopConfiguration(config);
        Assertions.assertEquals("stx-ha", conf.get("dfs.nameservices"));
        Assertions.assertEquals("nn-alpha,nn-beta", conf.get("dfs.ha.namenodes.stx-ha"));
        Assertions.assertEquals(
                "127.0.0.1:18020", conf.get("dfs.namenode.rpc-address.stx-ha.nn-alpha"));
        Assertions.assertEquals(
                "127.0.0.1:28020", conf.get("dfs.namenode.rpc-address.stx-ha.nn-beta"));
        Assertions.assertEquals("5000", conf.get("dfs.client.socket-timeout"));
        Assertions.assertTrue(conf.getBoolean("fs.hdfs.impl.disable.cache", false));

        Path resolved =
                StorageProbeConfigSupport.resolveTargetPath(
                        conf, config, "hdfs", "/seatunnel/checkpoint/", null, "list");
        Assertions.assertEquals("hdfs://stx-ha/seatunnel/checkpoint", resolved.toString());
    }

    @Test
    public void testBuildHadoopConfigurationRejectsMissingHdfsSitePath() {
        Map<String, String> config = new LinkedHashMap<>();
        config.put("hdfs_site_path", tempDir.resolve("missing-hdfs-site.xml").toString());

        ProxyException exception =
                Assertions.assertThrows(
                        ProxyException.class,
                        () -> StorageProbeConfigSupport.buildHadoopConfiguration(config));
        Assertions.assertEquals(400, exception.getStatusCode());
        Assertions.assertTrue(exception.getMessage().contains("hdfs_site_path"));
    }

    @Test
    public void testResolveTargetPathPreservesAlreadyQualifiedUri() {
        Map<String, String> config = new LinkedHashMap<>();
        config.put("storage.type", "s3");
        config.put("s3.bucket", "s3a://seatunnel-checkpoint");
        config.put("namespace", "/seatunnel/checkpoint/");

        Configuration conf = StorageProbeConfigSupport.buildHadoopConfiguration(config);
        Path s3Absolute =
                StorageProbeConfigSupport.resolveTargetPath(
                        conf,
                        config,
                        "s3",
                        "/seatunnel/checkpoint/",
                        "s3a://seatunnel-checkpoint/seatunnel/checkpoint/job-1/1.ser",
                        "preview");
        Assertions.assertEquals(
                "s3a://seatunnel-checkpoint/seatunnel/checkpoint/job-1/1.ser",
                s3Absolute.toString());

        Path s3Relative =
                StorageProbeConfigSupport.resolveTargetPath(
                        conf, config, "s3", "/seatunnel/checkpoint/", null, "list");
        Assertions.assertEquals(
                "s3a://seatunnel-checkpoint/seatunnel/checkpoint", s3Relative.toString());

        StorageProbeConfigSupport.applyCheckpointFastFailDefaults(config);
        Assertions.assertEquals("true", config.get("fs.s3a.impl.disable.cache"));
    }
}
