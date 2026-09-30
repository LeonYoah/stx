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

package io.github.leonyoah.stx.proxy.model;

import java.util.Collections;
import java.util.List;
import java.util.Map;

public class PluginOptionSchemaResult {

    private final boolean ok;
    private final String pluginType;
    private final String factoryIdentifier;
    private final List<PluginOptionDescriptor> options;
    /** 3.0 OptionRule 值约束；2.x 为空。 3.0 value constraints; empty on 2.x. */
    private final List<Map<String, Object>> valueConstraints;
    /** 3.0 条件子规则树；2.x 为空。 3.0 nested condition rules; empty on 2.x. */
    private final List<Map<String, Object>> conditionRules;

    private final List<String> warnings;

    public PluginOptionSchemaResult(
            boolean ok,
            String pluginType,
            String factoryIdentifier,
            List<PluginOptionDescriptor> options,
            List<String> warnings) {
        this(
                ok,
                pluginType,
                factoryIdentifier,
                options,
                Collections.emptyList(),
                Collections.emptyList(),
                warnings);
    }

    public PluginOptionSchemaResult(
            boolean ok,
            String pluginType,
            String factoryIdentifier,
            List<PluginOptionDescriptor> options,
            List<Map<String, Object>> valueConstraints,
            List<Map<String, Object>> conditionRules,
            List<String> warnings) {
        this.ok = ok;
        this.pluginType = pluginType;
        this.factoryIdentifier = factoryIdentifier;
        this.options = Collections.unmodifiableList(options);
        this.valueConstraints =
                Collections.unmodifiableList(
                        valueConstraints == null ? Collections.emptyList() : valueConstraints);
        this.conditionRules =
                Collections.unmodifiableList(
                        conditionRules == null ? Collections.emptyList() : conditionRules);
        this.warnings = Collections.unmodifiableList(warnings);
    }

    public boolean isOk() {
        return ok;
    }

    public String getPluginType() {
        return pluginType;
    }

    public String getFactoryIdentifier() {
        return factoryIdentifier;
    }

    public List<PluginOptionDescriptor> getOptions() {
        return options;
    }

    public List<Map<String, Object>> getValueConstraints() {
        return valueConstraints;
    }

    public List<Map<String, Object>> getConditionRules() {
        return conditionRules;
    }

    public List<String> getWarnings() {
        return warnings;
    }
}
