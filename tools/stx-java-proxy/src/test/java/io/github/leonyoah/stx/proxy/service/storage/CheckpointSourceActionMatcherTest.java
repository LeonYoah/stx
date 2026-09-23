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

import org.junit.jupiter.api.Test;

import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;

class CheckpointSourceActionMatcherTest {

    private final CheckpointSourceActionMatcher matcher = new CheckpointSourceActionMatcher();

    @Test
    void shouldMatchAllSourcesFromJobConfig() {
        Map<String, Object> request = new LinkedHashMap<>();
        Map<String, Object> jobConfig = new LinkedHashMap<>();
        jobConfig.put("contentFormat", "json");
        jobConfig.put(
                "content",
                "{\n"
                        + "  \"source\": [\n"
                        + "    { \"plugin_name\": \"MySQL-CDC\" },\n"
                        + "    { \"plugin_name\": \"Kafka\" }\n"
                        + "  ],\n"
                        + "  \"sink\": [\n"
                        + "    { \"plugin_name\": \"Console\" }\n"
                        + "  ]\n"
                        + "}");
        request.put("jobConfig", jobConfig);

        List<CheckpointSourceActionMatcher.SourceTarget> targets = matcher.match(request);

        assertEquals(2, targets.size());
        assertEquals("Source[0]-MySQL-CDC", targets.get(0).getActionName());
        assertEquals("Source[1]-Kafka", targets.get(1).getActionName());
    }

    @Test
    void shouldMatchSourcesWhenHoconContainsUnquotedMask() {
        Map<String, Object> request = new LinkedHashMap<>();
        Map<String, Object> jobConfig = new LinkedHashMap<>();
        jobConfig.put("contentFormat", "hocon");
        jobConfig.put(
                "content",
                "env {\n"
                        + "  job.mode = \"STREAMING\"\n"
                        + "}\n"
                        + "source {\n"
                        + "  MySQL-CDC {\n"
                        + "    plugin_name = \"MySQL-CDC\"\n"
                        + "    username = \"root\"\n"
                        + "    password = ******\n"
                        + "  }\n"
                        + "}\n"
                        + "sink {\n"
                        + "  Jdbc {\n"
                        + "    plugin_name = \"Jdbc\"\n"
                        + "    password = ******\n"
                        + "  }\n"
                        + "}\n");
        request.put("jobConfig", jobConfig);

        List<CheckpointSourceActionMatcher.SourceTarget> targets = matcher.match(request);

        assertEquals(1, targets.size());
        assertEquals("Source[0]-MySQL-CDC", targets.get(0).getActionName());
    }

    @Test
    void shouldRespectSourceTargetsSelection() {
        Map<String, Object> request = new LinkedHashMap<>();
        Map<String, Object> jobConfig = new LinkedHashMap<>();
        jobConfig.put("contentFormat", "json");
        jobConfig.put(
                "content",
                "{\n"
                        + "  \"source\": [\n"
                        + "    { \"plugin_name\": \"MySQL-CDC\" },\n"
                        + "    { \"plugin_name\": \"Http\" }\n"
                        + "  ],\n"
                        + "  \"sink\": [\n"
                        + "    { \"plugin_name\": \"Console\" }\n"
                        + "  ]\n"
                        + "}");
        request.put("jobConfig", jobConfig);
        request.put("sourceTargets", Collections.singletonList(1));

        List<CheckpointSourceActionMatcher.SourceTarget> targets = matcher.match(request);

        assertEquals(1, targets.size());
        assertEquals("Http", targets.get(0).getPluginName());
        assertEquals(1, targets.get(0).getConfigIndex());
    }
}
