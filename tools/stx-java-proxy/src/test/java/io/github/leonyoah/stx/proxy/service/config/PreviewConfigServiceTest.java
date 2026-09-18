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

import org.junit.jupiter.api.Assertions;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.Disabled;
import org.junit.jupiter.api.Test;

import io.github.leonyoah.stx.proxy.model.DagParseResult;
import io.github.leonyoah.stx.proxy.model.DatasetDag;
import io.github.leonyoah.stx.proxy.model.ProxyEdge;
import io.github.leonyoah.stx.proxy.model.ProxyNode;

import java.lang.reflect.Field;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;

public class PreviewConfigServiceTest {

    @BeforeAll
    static void setUpSeatunnelHome() {
        System.setProperty("SEATUNNEL_HOME", "/opt/seatunnel-2.3.13-new");
    }

    @Test
    public void testDeriveSourcePreviewFromJdbcGraph() {
        PreviewConfigService previewConfigService = previewServiceWithStubDag(1, 1, 1);
        Map<String, Object> request = new LinkedHashMap<>();
        request.put("content", jdbcSingleTableConfig());
        request.put("contentFormat", "hocon");
        request.put("httpSink", httpSink("https://proxy.example.com/source-preview"));
        request.put("platformJobId", "preview-1");
        request.put("engineJobId", "preview-1");
        request.put("previewRowLimit", 200);
        request.put("outputFormat", "json");

        Map<String, Object> response = previewConfigService.deriveSourcePreview(request);
        Assertions.assertEquals(Boolean.TRUE, response.get("ok"));

        String content = String.valueOf(response.get("content"));
        Assertions.assertTrue(content.contains("\"plugin_name\":\"Metadata\""));
        Assertions.assertTrue(content.contains("\"plugin_name\":\"Http\""));
        Assertions.assertTrue(content.contains("__st_preview_source_0"));
        Assertions.assertTrue(content.contains("platform_job_id=preview-1"));
        Assertions.assertTrue(content.contains("row_limit=200"));
        DatasetDag graph = (DatasetDag) response.get("graph");
        Assertions.assertNotNull(graph);
    }

    @Test
    @Disabled(
            "Manual JDBC transform scenario: depends on local MySQL fixture and transform plugin availability.")
    public void testDeriveTransformPreviewFromJdbcGraph() {
        PreviewConfigService previewConfigService = previewServiceWithStubDag(2, 2, 1);
        Map<String, Object> request = new LinkedHashMap<>();
        request.put(
                "content",
                String.join(
                        "\n",
                        "env {",
                        "  job.mode = \"batch\"",
                        "}",
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
                        "transform {",
                        "  Sql {",
                        "    plugin_input = [\"users_src\", \"orders_src\"]",
                        "    plugin_output = \"joined\"",
                        "    query = \"select * from users_src\"",
                        "  }",
                        "  Sql {",
                        "    plugin_input = [\"joined\"]",
                        "    plugin_output = \"cleaned\"",
                        "    query = \"select * from joined\"",
                        "  }",
                        "}",
                        "sink {",
                        "  Console {",
                        "    plugin_input = [\"cleaned\"]",
                        "  }",
                        "}"));
        request.put("contentFormat", "hocon");
        request.put("transformIndex", 0);
        request.put("httpSink", httpSink("https://proxy.example.com/transform-preview"));
        request.put("envOverrides", singletonMap("job.mode", "BATCH"));

        Map<String, Object> response = previewConfigService.deriveTransformPreview(request);
        Assertions.assertEquals(Boolean.TRUE, response.get("ok"));

        String content = String.valueOf(response.get("content"));
        Assertions.assertTrue(content.contains("\"plugin_name\":\"Sql\""));
        Assertions.assertTrue(content.contains("\"plugin_name\":\"Metadata\""));
        Assertions.assertTrue(content.contains("\"plugin_name\":\"Http\""));
        Assertions.assertFalse(content.contains("\"plugin_name\" : \"Console\""));
    }

    @Test
    public void testDeriveSourcePreviewAsHocon() {
        PreviewConfigService previewConfigService = previewServiceWithStubDag(1, 1, 1);
        Map<String, Object> request = new LinkedHashMap<>();
        request.put("content", jdbcSingleTableConfig());
        request.put("contentFormat", "hocon");
        request.put("httpSink", httpSink("https://proxy.example.com/source-preview"));
        request.put("outputFormat", "hocon");

        Map<String, Object> response = previewConfigService.deriveSourcePreview(request);

        Assertions.assertEquals("hocon", response.get("contentFormat"));
        String content = String.valueOf(response.get("content"));
        Assertions.assertTrue(content.contains("Metadata {"));
        Assertions.assertTrue(content.contains("Http {"));
    }

    private String jdbcSingleTableConfig() {
        return String.join(
                "\n",
                "env {",
                "  job.mode = \"batch\"",
                "}",
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
                "sink {",
                "  Jdbc {",
                "    plugin_input = [\"users_src\"]",
                "    url = \"jdbc:mysql://127.0.0.1:3307/demo2\"",
                "    username = \"root\"",
                "    password = \"seatunnel\"",
                "    driver = \"com.mysql.cj.jdbc.Driver\"",
                "    database = \"demo2\"",
                "    table = \"archive_${table_name}\"",
                "    generate_sink_sql = true",
                "  }",
                "}");
    }

    private Map<String, Object> httpSink(String url) {
        Map<String, Object> sink = new LinkedHashMap<>();
        sink.put("url", url);
        sink.put("array_mode", false);
        return sink;
    }

    private Map<String, Object> singletonMap(String key, Object value) {
        Map<String, Object> map = new LinkedHashMap<>();
        map.put(key, value);
        return map;
    }

    private PreviewConfigService previewServiceWithStubDag(
            int sourceCount, int transformCount, int sinkCount) {
        PreviewConfigService service = new PreviewConfigService();
        ConfigResourceService stubbedConfigResourceService =
                new ConfigResourceService() {
                    @Override
                    public DagParseResult inspectDag(Map<String, Object> request) {
                        return new DagParseResult(
                                true,
                                true,
                                sourceCount,
                                transformCount,
                                sinkCount,
                                Collections.emptyList(),
                                new DatasetDag(
                                        Collections.<ProxyNode>emptyList(),
                                        Collections.<ProxyEdge>emptyList()));
                    }
                };
        try {
            Field field = PreviewConfigService.class.getDeclaredField("configResourceService");
            field.setAccessible(true);
            field.set(service, stubbedConfigResourceService);
        } catch (ReflectiveOperationException e) {
            throw new RuntimeException("Unable to inject stubbed ConfigResourceService", e);
        }
        return service;
    }
}
