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

import org.apache.seatunnel.api.configuration.Option;
import org.apache.seatunnel.api.configuration.util.OptionRule;
import org.apache.seatunnel.api.configuration.util.RequiredOption;
import org.apache.seatunnel.api.table.factory.Factory;
import org.apache.seatunnel.api.table.factory.FactoryUtil;
import org.apache.seatunnel.api.table.factory.TableSinkFactory;
import org.apache.seatunnel.api.table.factory.TableSourceFactory;

import io.github.leonyoah.stx.proxy.model.OptionOrigin;
import io.github.leonyoah.stx.proxy.model.PluginOptionDescriptor;
import io.github.leonyoah.stx.proxy.model.RequiredMode;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

class FactoryOptionRuleExtractor {

    ExtractionResult extract(String pluginType, Factory factory) {
        List<String> warnings = new ArrayList<>();
        OptionRule optionRule = resolveOptionRule(pluginType, factory);
        if (optionRule == null) {
            warnings.add("factory.optionRule() returned null");
            return new ExtractionResult(new ArrayList<>(), warnings);
        }
        Map<String, PluginOptionDescriptor> descriptors = new LinkedHashMap<>();
        for (Option<?> option : optionRule.getOptionalOptions()) {
            PluginOptionDescriptor descriptor =
                    PluginOptionSupport.buildDescriptor(
                            option,
                            RequiredMode.OPTIONAL,
                            null,
                            null,
                            OptionOrigin.OPTION_RULE,
                            factory.getClass().getName(),
                            false);
            descriptors.put(option.key(), descriptor);
        }
        for (RequiredOption requiredOption : optionRule.getRequiredOptions()) {
            RequiredMode requiredMode = resolveRequiredMode(requiredOption);
            String conditionExpression =
                    requiredOption instanceof RequiredOption.ConditionalRequiredOptions
                            ? ((RequiredOption.ConditionalRequiredOptions) requiredOption)
                                    .getExpression()
                                    .toString()
                            : null;
            String constraintGroup = requiredOption.toString();
            for (Option<?> option : requiredOption.getOptions()) {
                PluginOptionDescriptor descriptor =
                        PluginOptionSupport.buildDescriptor(
                                option,
                                requiredMode,
                                conditionExpression,
                                constraintGroup,
                                OptionOrigin.OPTION_RULE,
                                factory.getClass().getName(),
                                false);
                PluginOptionDescriptor existing = descriptors.get(option.key());
                if (existing == null) {
                    descriptors.put(option.key(), descriptor);
                } else {
                    existing.setRequiredMode(requiredMode);
                    existing.setConditionExpression(conditionExpression);
                    existing.setConstraintGroup(constraintGroup);
                    PluginOptionSupport.merge(existing, descriptor);
                }
            }
        }
        if (descriptors.isEmpty()) {
            warnings.add("factory.optionRule() is empty");
        }
        return new ExtractionResult(new ArrayList<>(descriptors.values()), warnings);
    }

    private OptionRule resolveOptionRule(String pluginType, Factory factory) {
        switch (pluginType) {
            case "source":
                return FactoryUtil.sourceFullOptionRule((TableSourceFactory) factory);
            case "sink":
                return FactoryUtil.sinkFullOptionRule((TableSinkFactory) factory);
            case "transform":
            case "catalog":
                return factory.optionRule();
            default:
                throw new ProxyException(400, "Unsupported plugin type: " + pluginType);
        }
    }

    private RequiredMode resolveRequiredMode(RequiredOption requiredOption) {
        if (requiredOption instanceof RequiredOption.AbsolutelyRequiredOptions) {
            return RequiredMode.REQUIRED;
        }
        if (requiredOption instanceof RequiredOption.ExclusiveRequiredOptions) {
            return RequiredMode.EXCLUSIVE;
        }
        if (requiredOption instanceof RequiredOption.ConditionalRequiredOptions) {
            return RequiredMode.CONDITIONAL;
        }
        if (requiredOption instanceof RequiredOption.BundledRequiredOptions) {
            return RequiredMode.BUNDLED;
        }
        return RequiredMode.REQUIRED;
    }

    static class ExtractionResult {
        private final List<PluginOptionDescriptor> options;
        private final List<String> warnings;

        ExtractionResult(List<PluginOptionDescriptor> options, List<String> warnings) {
            this.options = options;
            this.warnings = warnings;
        }

        List<PluginOptionDescriptor> getOptions() {
            return options;
        }

        List<String> getWarnings() {
            return warnings;
        }
    }
}
