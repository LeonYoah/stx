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
import org.junit.jupiter.api.Test;

import io.github.leonyoah.stx.proxy.model.DagParseResult;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;

public class ConfigResourceServiceTest {

    private final ConfigResourceService service = new ConfigResourceService();

    @Test
    public void testSimpleGraphCompatibilityEdge() {
        Map<String, Object> request = new LinkedHashMap<>();
        request.put(
                "content",
                "source {\n"
                        + "  FakeSource {\n"
                        + "  }\n"
                        + "}\n"
                        + "sink {\n"
                        + "  Console {\n"
                        + "    plugin_input = [\"preview_rows\"]\n"
                        + "  }\n"
                        + "}\n");
        request.put("contentFormat", "hocon");

        DagParseResult result = service.inspectDag(request);

        Assertions.assertTrue(result.isOk());
        Assertions.assertTrue(result.isSimpleGraph());
        Assertions.assertEquals(2, result.getGraph().getNodes().size());
        Assertions.assertEquals(1, result.getGraph().getEdges().size());
        Assertions.assertEquals(
                "preview_rows", result.getGraph().getEdges().get(0).getFromDataset());
        Assertions.assertFalse(result.getWarnings().isEmpty());
    }

    @Test
    public void testComplexGraphMultiInputDag() {
        Map<String, Object> request = new LinkedHashMap<>();
        request.put(
                "content",
                "source {\n"
                        + "  SourceA {\n"
                        + "    plugin_output = \"a\"\n"
                        + "  }\n"
                        + "  SourceB {\n"
                        + "    plugin_output = \"b\"\n"
                        + "  }\n"
                        + "}\n"
                        + "transform {\n"
                        + "  Joiner {\n"
                        + "    plugin_input = [\"a\", \"b\"]\n"
                        + "    plugin_output = \"joined\"\n"
                        + "  }\n"
                        + "}\n"
                        + "sink {\n"
                        + "  Console {\n"
                        + "    plugin_input = [\"joined\"]\n"
                        + "  }\n"
                        + "}\n");
        request.put("contentFormat", "hocon");

        DagParseResult result = service.inspectDag(request);

        Assertions.assertFalse(result.isSimpleGraph());
        Assertions.assertEquals(4, result.getGraph().getNodes().size());
        Assertions.assertEquals(3, result.getGraph().getEdges().size());
    }

    @Test
    public void testMissingContentAndFilePath() {
        ProxyException exception =
                Assertions.assertThrows(
                        ProxyException.class, () -> service.inspectDag(Collections.emptyMap()));
        Assertions.assertEquals(400, exception.getStatusCode());
    }
}
