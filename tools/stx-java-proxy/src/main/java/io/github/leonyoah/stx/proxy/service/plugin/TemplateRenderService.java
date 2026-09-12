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

import io.github.leonyoah.stx.proxy.model.PluginOptionDescriptor;
import io.github.leonyoah.stx.proxy.model.PluginOptionSchemaResult;
import io.github.leonyoah.stx.proxy.model.PluginTemplateResult;
import io.github.leonyoah.stx.proxy.model.RequiredMode;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.Map;

public class TemplateRenderService {

    private final PluginOptionSchemaService pluginOptionSchemaService;

    public TemplateRenderService() {
        this(new PluginOptionSchemaService());
    }

    TemplateRenderService(PluginOptionSchemaService pluginOptionSchemaService) {
        this.pluginOptionSchemaService = pluginOptionSchemaService;
    }

    public PluginTemplateResult render(Map<String, Object> request) {
        boolean includeAdvanced = ProxyRequestUtils.getBoolean(request, "includeAdvanced", false);
        PluginOptionSchemaResult schema = pluginOptionSchemaService.inspect(request);
        List<PluginOptionDescriptor> requiredOptions = new ArrayList<>();
        List<PluginOptionDescriptor> conditionalOptions = new ArrayList<>();
        List<PluginOptionDescriptor> optionalOptions = new ArrayList<>();
        for (PluginOptionDescriptor option : schema.getOptions()) {
            if (!includeAdvanced && option.isAdvanced()) {
                continue;
            }
            if (isConditionallyRequired(option)) {
                conditionalOptions.add(option);
            } else if (isRequired(option)) {
                requiredOptions.add(option);
            } else {
                optionalOptions.add(option);
            }
        }
        requiredOptions.sort(Comparator.comparing(PluginOptionDescriptor::getKey));
        conditionalOptions.sort(Comparator.comparing(PluginOptionDescriptor::getKey));
        optionalOptions.sort(Comparator.comparing(PluginOptionDescriptor::getKey));

        List<String> lines = new ArrayList<>();
        lines.add(schema.getFactoryIdentifier() + " {");
        if ("sink".equalsIgnoreCase(schema.getPluginType())
                || "transform".equalsIgnoreCase(schema.getPluginType())) {
            lines.add("  # input (compatible with source_table_name)");
            lines.add("  plugin_input = []");
        }
        if (!requiredOptions.isEmpty()) {
            lines.add("  # required");
            for (PluginOptionDescriptor option : requiredOptions) {
                lines.add("  " + option.getKey() + " = " + PluginOptionSupport.renderValue(option));
            }
        }
        if (!conditionalOptions.isEmpty()) {
            lines.add("  # conditional");
            for (PluginOptionDescriptor option : conditionalOptions) {
                String renderedValue = PluginOptionSupport.renderValue(option);
                String renderedLine = option.getKey() + " = " + renderedValue;
                lines.add(
                        shouldCommentOutOptionalValue(renderedValue)
                                ? "  # " + renderedLine
                                : "  " + renderedLine);
            }
        }
        if (!optionalOptions.isEmpty()) {
            lines.add("  # optional");
            for (PluginOptionDescriptor option : optionalOptions) {
                String renderedValue = PluginOptionSupport.renderValue(option);
                String renderedLine = option.getKey() + " = " + renderedValue;
                lines.add(
                        shouldCommentOutOptionalValue(renderedValue)
                                ? "  # " + renderedLine
                                : "  " + renderedLine);
            }
        }
        if ("source".equalsIgnoreCase(schema.getPluginType())
                || "transform".equalsIgnoreCase(schema.getPluginType())) {
            lines.add("  # output (compatible with result_table_name)");
            lines.add("  plugin_output = \"\"");
        }
        lines.add("}");
        return new PluginTemplateResult(
                true,
                schema.getPluginType(),
                schema.getFactoryIdentifier(),
                "hocon",
                String.join("\n", lines),
                schema.getWarnings());
    }

    private boolean isRequired(PluginOptionDescriptor option) {
        if (option == null || option.getRequiredMode() == null) {
            return false;
        }
        RequiredMode mode = option.getRequiredMode();
        return RequiredMode.REQUIRED.equals(mode)
                || RequiredMode.BUNDLED.equals(mode)
                || RequiredMode.EXCLUSIVE.equals(mode);
    }

    private boolean isConditionallyRequired(PluginOptionDescriptor option) {
        return option != null && RequiredMode.CONDITIONAL.equals(option.getRequiredMode());
    }

    private boolean shouldCommentOutOptionalValue(String renderedValue) {
        return "\"\"".equals(renderedValue)
                || "null".equals(renderedValue)
                || "[]".equals(renderedValue)
                || "{}".equals(renderedValue);
    }
}
