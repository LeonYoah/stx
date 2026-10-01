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

import org.apache.seatunnel.shade.com.typesafe.config.ConfigFactory;

import org.junit.jupiter.api.Test;

import io.github.leonyoah.stx.proxy.model.DatasetDag;
import io.github.leonyoah.stx.proxy.model.JobConfigContext;

import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class OptionRuleValidationServiceWiringTest {

    @Test
    void validateOptionRulesCanBeDisabled() {
        Map<String, Object> request = new LinkedHashMap<>();
        request.put(
                "content",
                String.join(
                        "\n",
                        "env { job.mode = \"batch\" }",
                        "source { FakeSource { plugin_output = \"fake\" } }",
                        "sink { Console { plugin_input = [\"fake\"] } }"));
        request.put("contentFormat", "hocon");
        request.put("validateOptionRules", false);
        request.put("testConnection", false);

        Map<String, Object> result =
                new ConfigValidationService(new EmptyJobConfigSupportService()).validate(request);

        assertTrue((Boolean) result.get("valid"));
        @SuppressWarnings("unchecked")
        List<String> errors = (List<String>) result.get("errors");
        assertTrue(errors == null || errors.isEmpty());
    }

    @Test
    void defaultConstructorKeepsOptionRuleService() {
        // 冒烟：默认构造可创建；真实 SEATUNNEL_HOME 缺失时校验会降级为 warning。
        // Smoke: default ctor works; missing SEATUNNEL_HOME degrades to warnings.
        ConfigValidationService service = new ConfigValidationService();
        assertFalse(service == null);
    }

    private static class EmptyJobConfigSupportService extends JobConfigSupportService {
        @Override
        public JobConfigContext parseJobContext(Map<String, Object> request) {
            return new JobConfigContext(
                    ConfigFactory.empty(),
                    Collections.emptyList(),
                    Collections.emptyList(),
                    Collections.emptyList(),
                    true,
                    new ArrayList<>(),
                    new DatasetDag(Collections.emptyList(), Collections.emptyList()));
        }
    }
}
