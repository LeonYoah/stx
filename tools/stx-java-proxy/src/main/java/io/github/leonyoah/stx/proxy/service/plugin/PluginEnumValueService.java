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

import org.apache.seatunnel.shade.org.apache.commons.lang3.StringUtils;

import io.github.leonyoah.stx.proxy.model.PluginEnumValuesResult;
import io.github.leonyoah.stx.proxy.model.PluginOptionDescriptor;
import io.github.leonyoah.stx.proxy.model.PluginOptionSchemaResult;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.util.ArrayList;
import java.util.Map;

public class PluginEnumValueService {

    private final PluginOptionSchemaService pluginOptionSchemaService;

    public PluginEnumValueService() {
        this(new PluginOptionSchemaService());
    }

    PluginEnumValueService(PluginOptionSchemaService pluginOptionSchemaService) {
        this.pluginOptionSchemaService = pluginOptionSchemaService;
    }

    public PluginEnumValuesResult listValues(Map<String, Object> request) {
        String optionKey = ProxyRequestUtils.getRequiredString(request, "optionKey");
        PluginOptionSchemaResult schema = pluginOptionSchemaService.inspect(request);
        for (PluginOptionDescriptor option : schema.getOptions()) {
            if (StringUtils.equals(option.getKey(), optionKey)) {
                return new PluginEnumValuesResult(
                        true,
                        schema.getPluginType(),
                        schema.getFactoryIdentifier(),
                        optionKey,
                        option.getEnumValues(),
                        schema.getWarnings());
            }
        }
        return new PluginEnumValuesResult(
                true,
                schema.getPluginType(),
                schema.getFactoryIdentifier(),
                optionKey,
                new ArrayList<>(),
                schema.getWarnings());
    }
}
