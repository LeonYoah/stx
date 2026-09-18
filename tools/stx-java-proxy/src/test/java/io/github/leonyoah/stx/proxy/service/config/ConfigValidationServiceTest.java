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

package io.github.leonyoah.stx.proxy.service.config;

import org.apache.seatunnel.shade.com.typesafe.config.Config;
import org.apache.seatunnel.shade.com.typesafe.config.ConfigFactory;

import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Disabled;
import org.junit.jupiter.api.Test;

import io.github.leonyoah.stx.proxy.model.DatasetDag;
import io.github.leonyoah.stx.proxy.model.JobConfigContext;
import io.github.leonyoah.stx.proxy.model.NodeKind;
import io.github.leonyoah.stx.proxy.model.ProxyEdge;
import io.github.leonyoah.stx.proxy.model.ProxyNode;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.util.Arrays;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class ConfigValidationServiceTest {

    @BeforeAll
    static void setUpSeatunnelHome() {
        System.setProperty("SEATUNNEL_HOME", "/opt/seatunnel-2.3.13-new");
    }

    private final ConfigValidationService service = new ConfigValidationService();

    @Test
    void testConnectionSkippedDoesNotPass() {
        Map<String, Object> request = new LinkedHashMap<>();
        request.put(
                "content",
                String.join(
                        "\n",
                        "env {",
                        "  job.mode = \"batch\"",
                        "}",
                        "source {",
                        "  Http {",
                        "    plugin_output = \"src\"",
                        "    url = \"http://127.0.0.1:8080\"",
                        "  }",
                        "}",
                        "sink {",
                        "  Console {",
                        "    plugin_input = [\"src\"]",
                        "  }",
                        "}"));
        request.put("contentFormat", "hocon");
        request.put("testConnection", true);

        Map<String, Object> result =
                new ConfigValidationService(new FakeJobConfigSupportService()).validate(request);

        assertFalse((Boolean) result.get("ok"));
        assertFalse((Boolean) result.get("valid"));
        @SuppressWarnings("unchecked")
        List<String> warnings = (List<String>) result.get("warnings");
        assertTrue(warnings.stream().anyMatch(item -> item.contains("暂未完成连接测试")));
        assertFalse(warnings.stream().anyMatch(item -> item.contains("Console")));
    }

    @Test
    void fakeSourceAndConsoleConnectionSkippedAsInfo() {
        Map<String, Object> request = new LinkedHashMap<>();
        request.put(
                "content",
                String.join(
                        "\n",
                        "env {",
                        "  job.mode = \"batch\"",
                        "}",
                        "source {",
                        "  FakeSource {",
                        "    plugin_output = \"fake\"",
                        "  }",
                        "}",
                        "sink {",
                        "  Console {",
                        "    plugin_input = [\"fake\"]",
                        "  }",
                        "}"));
        request.put("contentFormat", "hocon");
        request.put("testConnection", true);

        Map<String, Object> result =
                new ConfigValidationService(new FakeJobConfigSupportService()).validate(request);

        assertTrue((Boolean) result.get("ok"));
        assertTrue((Boolean) result.get("valid"));
        @SuppressWarnings("unchecked")
        List<String> warnings = (List<String>) result.get("warnings");
        @SuppressWarnings("unchecked")
        List<String> infos = (List<String>) result.get("infos");
        assertTrue(warnings.isEmpty());
        assertTrue(infos.stream().anyMatch(item -> item.contains("FakeSource 无需连接测试")));
        assertTrue(infos.stream().anyMatch(item -> item.contains("Console 无需连接测试")));
    }

    @Test
    void invalidConnectorShouldFailFast() {
        Map<String, Object> request = new LinkedHashMap<>();
        request.put(
                "content",
                String.join(
                        "\n",
                        "env {",
                        "  job.mode = \"batch\"",
                        "}",
                        "source {",
                        "  JDBC2 {",
                        "    plugin_output = \"src\"",
                        "  }",
                        "}",
                        "sink {",
                        "  Console {",
                        "    plugin_input = [\"src\"]",
                        "  }",
                        "}"));
        request.put("contentFormat", "hocon");

        try {
            service.validate(request);
        } catch (ProxyException e) {
            assertEquals(400, e.getStatusCode());
            assertTrue(e.getMessage().contains("JDBC2 连接器未安装"));
            return;
        }
        throw new AssertionError("expected ProxyException");
    }

    @Test
    @Disabled("Manual stress scenario: requires local MySQL fixture on 127.0.0.1:3307")
    void hundredJdbcTablesConnectionStressScenario() {
        Map<String, Object> request = new LinkedHashMap<>();
        request.put("content", buildHundredTableJdbcConfig());
        request.put("contentFormat", "hocon");
        request.put("testConnection", true);

        Map<String, Object> result = service.validate(request);

        assertTrue(result.containsKey("checks"));
    }

    private String buildHundredTableJdbcConfig() {
        StringBuilder builder = new StringBuilder();
        builder.append("env {\n  job.mode = \"batch\"\n}\n\n");
        builder.append("source {\n  Jdbc {\n");
        builder.append("    plugin_output = \"bulk_src\"\n");
        builder.append("    url = \"jdbc:mysql://127.0.0.1:3307/seatunnel_demo\"\n");
        builder.append("    username = \"root\"\n");
        builder.append("    password = \"seatunnel\"\n");
        builder.append("    driver = \"com.mysql.cj.jdbc.Driver\"\n");
        builder.append("    table_list = [\n");
        for (int i = 1; i <= 100; i++) {
            builder.append(
                    String.format("      { table_path = \"seatunnel_demo.table_%03d\" }", i));
            builder.append(i == 100 ? "\n" : ",\n");
        }
        builder.append("    ]\n  }\n}\n\n");
        builder.append("sink {\n  Jdbc {\n");
        builder.append("    plugin_input = [\"bulk_src\"]\n");
        builder.append("    url = \"jdbc:mysql://127.0.0.1:3307/demo2\"\n");
        builder.append("    username = \"root\"\n");
        builder.append("    password = \"seatunnel\"\n");
        builder.append("    driver = \"com.mysql.cj.jdbc.Driver\"\n");
        builder.append("    database = \"demo2\"\n");
        builder.append("    table = \"archive_${table_name}\"\n");
        builder.append("    generate_sink_sql = true\n");
        builder.append("  }\n}\n");
        return builder.toString();
    }

    private static class FakeJobConfigSupportService extends JobConfigSupportService {
        @Override
        public JobConfigContext parseJobContext(Map<String, Object> request) {
            Config sourceConfig =
                    ConfigFactory.parseString(
                            "plugin_name = \""
                                    + (String.valueOf(request.get("content")).contains("FakeSource")
                                            ? "FakeSource"
                                            : "Http")
                                    + "\"\n"
                                    + "plugin_output = \"fake\"\n"
                                    + "url = \"http://127.0.0.1:8080\"\n");
            Config sinkConfig =
                    ConfigFactory.parseString(
                            "plugin_name = \"Console\"\n" + "plugin_input = [\"fake\"]\n");
            List<ProxyNode> nodes =
                    Arrays.asList(
                            new ProxyNode(
                                    "source-0",
                                    NodeKind.SOURCE,
                                    sourceConfig.getString("plugin_name"),
                                    0,
                                    Collections.emptyList(),
                                    "fake",
                                    Collections.emptyList(),
                                    Collections.emptyMap(),
                                    Collections.emptyMap()),
                            new ProxyNode(
                                    "sink-0",
                                    NodeKind.SINK,
                                    "Console",
                                    0,
                                    Collections.singletonList("fake"),
                                    "",
                                    Collections.emptyList(),
                                    Collections.emptyMap(),
                                    Collections.emptyMap()));
            return new JobConfigContext(
                    ConfigFactory.empty(),
                    Collections.singletonList(sourceConfig),
                    Collections.emptyList(),
                    Collections.singletonList(sinkConfig),
                    true,
                    Collections.singletonList(
                            "Simple graph compatibility linked FakeSource to Console by source/transform output 'fake'."),
                    new DatasetDag(
                            nodes,
                            Collections.singletonList(
                                    new ProxyEdge("fake", "source-0", "sink-0"))));
        }
    }
}
