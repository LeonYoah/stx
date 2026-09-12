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

import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Disabled;
import org.junit.jupiter.api.Test;

import io.github.leonyoah.stx.proxy.model.WebUiDagPreviewResult;
import io.github.leonyoah.stx.proxy.model.WebUiDagVertexInfo;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.util.Arrays;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertIterableEquals;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

class WebUiDagPreviewServiceTest {

    @BeforeAll
    static void setUpSeatunnelHome() {
        System.setProperty("SEATUNNEL_HOME", "/opt/seatunnel-2.3.13-new");
    }

    private final WebUiDagPreviewService service = new WebUiDagPreviewService();

    @Test
    void previewBuildsWebUiCompatibleDag() {
        WebUiDagPreviewResult result =
                preview(
                        conf(
                                "env {",
                                "  job.mode = \"batch\"",
                                "}",
                                "",
                                "source {",
                                "  Jdbc {",
                                "    plugin_output = \"users_src\"",
                                "    url = \"jdbc:mysql://127.0.0.1:3307/seatunnel_demo\"",
                                "    username = \"root\"",
                                "    password = \"seatunnel\"",
                                "    driver = \"com.mysql.cj.jdbc.Driver\"",
                                "    table_path = \"seatunnel_demo.users\"",
                                "  }",
                                "}",
                                "",
                                "sink {",
                                "  Console {",
                                "    plugin_input = [\"users_src\"]",
                                "  }",
                                "}"));

        assertEquals("preview", result.getJobId());
        assertEquals("Config Preview", result.getJobName());
        assertEquals("CREATED", result.getJobStatus());
        assertNotNull(result.getJobDag());
        assertEquals(1, result.getJobDag().getPipelineEdges().size());
        assertEquals(2, result.getJobDag().getVertexInfoMap().size());
        assertEquals("Source[0]-Jdbc", vertex(result, 1).getConnectorType());
        assertEquals("Sink[0]-Console", vertex(result, 2).getConnectorType());
        assertIterableEquals(
                Collections.singletonList("seatunnel_demo.users"),
                vertex(result, 1).getTablePaths());
        assertIterableEquals(
                Arrays.asList("id", "name"),
                vertex(result, 1).getTableColumns().get("seatunnel_demo.users"));
        assertIterableEquals(
                Collections.singletonList("seatunnel_demo.users"),
                vertex(result, 2).getTablePaths());
        assertEquals(1, result.getJobDag().getPipelineEdges().get(0).size());
        assertFalse(result.getMetrics().isEmpty());
    }

    @Test
    void previewFailsFastWhenOfficialConnectorInitializationFails() {
        try {
            preview(
                    conf(
                            "env {",
                            "  job.mode = \"batch\"",
                            "}",
                            "",
                            "source {",
                            "  Icberg {",
                            "    plugin_output = \"orders\"",
                            "  }",
                            "}",
                            "",
                            "sink {",
                            "  Console {",
                            "    plugin_input = [\"orders\"]",
                            "  }",
                            "}"));
        } catch (ProxyException e) {
            assertTrue(e.getMessage().contains("official tablePath resolution failed"));
            return;
        }
        throw new AssertionError("expected ProxyException");
    }

    @Test
    void previewUsesOfficialJdbcSingleTableAndSinkPlaceholder() {
        WebUiDagPreviewResult result =
                preview(
                        conf(
                                "env {",
                                "  job.mode = \"batch\"",
                                "}",
                                "",
                                "source {",
                                "  Jdbc {",
                                "    plugin_output = \"users_src\"",
                                "    url = \"jdbc:mysql://127.0.0.1:3307/seatunnel_demo\"",
                                "    username = \"root\"",
                                "    password = \"seatunnel\"",
                                "    driver = \"com.mysql.cj.jdbc.Driver\"",
                                "    table_path = \"seatunnel_demo.users\"",
                                "  }",
                                "}",
                                "",
                                "sink {",
                                "  Jdbc {",
                                "    plugin_input = [\"users_src\"]",
                                "    url = \"jdbc:mysql://127.0.0.1:3307/demo2\"",
                                "    username = \"root\"",
                                "    password = \"seatunnel\"",
                                "    driver = \"com.mysql.cj.jdbc.Driver\"",
                                "    database = \"demo2\"",
                                "    table = \"${table_name}\"",
                                "    generate_sink_sql = true",
                                "  }",
                                "}"));

        assertIterableEquals(
                Collections.singletonList("seatunnel_demo.users"),
                vertex(result, 1).getTablePaths());
        assertIterableEquals(
                Arrays.asList("id", "name"),
                vertex(result, 1).getTableColumns().get("seatunnel_demo.users"));
        assertIterableEquals(
                Collections.singletonList("demo2.users"), vertex(result, 2).getTablePaths());
    }

    @Test
    void previewUsesOfficialJdbcMultiTableAndSinkPlaceholder() {
        WebUiDagPreviewResult result =
                preview(
                        conf(
                                "env {",
                                "  job.mode = \"batch\"",
                                "}",
                                "",
                                "source {",
                                "  Jdbc {",
                                "    plugin_output = \"mysql_src\"",
                                "    url = \"jdbc:mysql://127.0.0.1:3307/seatunnel_demo\"",
                                "    username = \"root\"",
                                "    password = \"seatunnel\"",
                                "    driver = \"com.mysql.cj.jdbc.Driver\"",
                                "    table_list = [",
                                "      { table_path = \"seatunnel_demo.users\" },",
                                "      { table_path = \"seatunnel_demo.orders\" }",
                                "    ]",
                                "  }",
                                "}",
                                "",
                                "sink {",
                                "  Jdbc {",
                                "    plugin_input = [\"mysql_src\"]",
                                "    url = \"jdbc:mysql://127.0.0.1:3307/demo2\"",
                                "    username = \"root\"",
                                "    password = \"seatunnel\"",
                                "    driver = \"com.mysql.cj.jdbc.Driver\"",
                                "    database = \"demo2\"",
                                "    table = \"archive_${table_name}\"",
                                "    generate_sink_sql = true",
                                "  }",
                                "}"));

        assertIterableEquals(
                Arrays.asList("seatunnel_demo.users", "seatunnel_demo.orders"),
                vertex(result, 1).getTablePaths());
        assertIterableEquals(
                Arrays.asList("id", "name"),
                vertex(result, 1).getTableColumns().get("seatunnel_demo.users"));
        assertIterableEquals(
                Arrays.asList("id", "amount"),
                vertex(result, 1).getTableColumns().get("seatunnel_demo.orders"));
        assertIterableEquals(
                Arrays.asList("demo2.archive_users", "demo2.archive_orders"),
                vertex(result, 2).getTablePaths());
    }

    @Test
    void previewSupportsMultiSourceAndMultiSinkPaths() {
        WebUiDagPreviewResult result =
                preview(
                        conf(
                                "env {",
                                "  job.mode = \"batch\"",
                                "}",
                                "",
                                "source {",
                                "  Jdbc {",
                                "    plugin_output = \"users_src\"",
                                "    url = \"jdbc:mysql://127.0.0.1:3307/seatunnel_demo\"",
                                "    username = \"root\"",
                                "    password = \"seatunnel\"",
                                "    driver = \"com.mysql.cj.jdbc.Driver\"",
                                "    table_path = \"seatunnel_demo.users\"",
                                "  }",
                                "",
                                "  Jdbc {",
                                "    plugin_output = \"orders_src\"",
                                "    url = \"jdbc:mysql://127.0.0.1:3307/seatunnel_demo\"",
                                "    username = \"root\"",
                                "    password = \"seatunnel\"",
                                "    driver = \"com.mysql.cj.jdbc.Driver\"",
                                "    table_path = \"seatunnel_demo.orders\"",
                                "  }",
                                "}",
                                "",
                                "sink {",
                                "  Console {",
                                "    plugin_input = [\"users_src\", \"orders_src\"]",
                                "  }",
                                "",
                                "  Jdbc {",
                                "    plugin_input = [\"users_src\"]",
                                "    url = \"jdbc:mysql://127.0.0.1:3307/demo2\"",
                                "    username = \"root\"",
                                "    password = \"seatunnel\"",
                                "    driver = \"com.mysql.cj.jdbc.Driver\"",
                                "    database = \"demo2\"",
                                "    table = \"archive_users\"",
                                "    generate_sink_sql = true",
                                "  }",
                                "}"));

        assertIterableEquals(
                Collections.singletonList("seatunnel_demo.users"),
                vertexByConnector(result, "Source[0]-Jdbc").getTablePaths());
        assertIterableEquals(
                Collections.singletonList("seatunnel_demo.orders"),
                vertexByConnector(result, "Source[1]-Jdbc").getTablePaths());
        assertIterableEquals(
                Arrays.asList("seatunnel_demo.users", "seatunnel_demo.orders"),
                vertexByConnector(result, "Sink[0]-Console").getTablePaths());
        assertIterableEquals(
                Collections.singletonList("demo2.archive_users"),
                vertexByConnector(result, "Sink[1]-Jdbc").getTablePaths());
        assertEquals(4, result.getJobDag().getVertexInfoMap().size());
    }

    @Test
    @Disabled("Manual stress scenario: requires local MySQL fixture with 100 tables")
    void previewHandlesHundredJdbcTables() {
        WebUiDagPreviewResult result = preview(buildHundredTableJdbcConfig());

        Set<String> sourcePaths =
                new LinkedHashSet<>(vertexByConnector(result, "Source[0]-Jdbc").getTablePaths());
        Set<String> sinkPaths =
                new LinkedHashSet<>(vertexByConnector(result, "Sink[0]-Jdbc").getTablePaths());

        assertEquals(100, sourcePaths.size());
        assertEquals(100, sinkPaths.size());
        assertTrue(sourcePaths.contains("seatunnel_demo.table_001"));
        assertTrue(sourcePaths.contains("seatunnel_demo.table_100"));
        assertTrue(sinkPaths.contains("demo2.archive_table_001"));
        assertTrue(sinkPaths.contains("demo2.archive_table_100"));
    }

    private WebUiDagPreviewResult preview(String content) {
        return service.preview(previewRequest(content));
    }

    private java.util.Map<String, Object> previewRequest(String content) {
        LinkedHashMap<String, Object> request = new LinkedHashMap<>();
        request.put("content", content);
        request.put("contentFormat", "hocon");
        return request;
    }

    private String conf(String... lines) {
        return String.join("\n", lines);
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

    private WebUiDagVertexInfo vertex(WebUiDagPreviewResult result, int vertexId) {
        return result.getJobDag().getVertexInfoMap().get(vertexId);
    }

    private WebUiDagVertexInfo vertexByConnector(
            WebUiDagPreviewResult result, String connectorType) {
        return result.getJobDag().getVertexInfoMap().values().stream()
                .filter(vertex -> connectorType.equals(vertex.getConnectorType()))
                .findFirst()
                .orElseThrow(() -> new AssertionError("missing connector: " + connectorType));
    }
}
