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

import org.junit.jupiter.api.Test;

import io.github.leonyoah.stx.proxy.model.OptionOrigin;
import io.github.leonyoah.stx.proxy.model.PluginOptionDescriptor;
import io.github.leonyoah.stx.proxy.model.PluginOptionSchemaResult;
import io.github.leonyoah.stx.proxy.model.RequiredMode;

import java.util.Arrays;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class TemplateRenderServiceTest {

    @Test
    void shouldRenderCompactPluginBlockAndSkipAdvancedOptionsByDefault() {
        PluginOptionDescriptor required = new PluginOptionDescriptor();
        required.setKey("url");
        required.setType("string");
        required.setDescription("Jdbc url");
        required.setRequiredMode(RequiredMode.REQUIRED);
        required.setOrigins(Collections.singletonList(OptionOrigin.OPTION_RULE));
        required.setAdvanced(false);

        PluginOptionDescriptor advanced = new PluginOptionDescriptor();
        advanced.setKey("save_mode");
        advanced.setType("string");
        advanced.setDescription("Save mode");
        advanced.setEnumValues(Arrays.asList("APPEND", "OVERWRITE"));
        advanced.setRequiredMode(RequiredMode.SUPPLEMENTAL_OPTIONAL);
        advanced.setOrigins(Collections.singletonList(OptionOrigin.FIELD_SCAN));
        advanced.setAdvanced(true);

        PluginOptionDescriptor optionalEmpty = new PluginOptionDescriptor();
        optionalEmpty.setKey("table_list");
        optionalEmpty.setType("list");
        optionalEmpty.setDefaultValue(Collections.emptyList());
        optionalEmpty.setRequiredMode(RequiredMode.OPTIONAL);
        optionalEmpty.setOrigins(Collections.singletonList(OptionOrigin.OPTION_RULE));
        optionalEmpty.setAdvanced(false);

        PluginOptionSchemaResult schema =
                new PluginOptionSchemaResult(
                        true,
                        "sink",
                        "Jdbc",
                        Arrays.asList(required, advanced, optionalEmpty),
                        Collections.singletonList("rule warning"));
        TemplateRenderService service =
                new TemplateRenderService(new StubPluginOptionSchemaService(schema));

        Map<String, Object> request = new LinkedHashMap<>();
        request.put("pluginType", "sink");
        request.put("factoryIdentifier", "Jdbc");
        String template = service.render(request).getTemplate();

        assertTrue(template.startsWith("Jdbc {"));
        assertTrue(template.contains("# input"));
        assertTrue(template.contains("# required"));
        assertTrue(template.contains("plugin_input = []"));
        assertTrue(template.contains("url = \"\""));
        assertTrue(template.contains("# table_list = []"));
        assertFalse(template.contains("save_mode"));
        assertFalse(template.contains("\n\n"));
    }

    private static class StubPluginOptionSchemaService extends PluginOptionSchemaService {
        private final PluginOptionSchemaResult schema;

        private StubPluginOptionSchemaService(PluginOptionSchemaResult schema) {
            this.schema = schema;
        }

        @Override
        public PluginOptionSchemaResult inspect(Map<String, Object> request) {
            return schema;
        }
    }
}
