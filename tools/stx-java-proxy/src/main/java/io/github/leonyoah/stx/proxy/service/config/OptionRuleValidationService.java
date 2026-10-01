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

import org.apache.seatunnel.shade.com.typesafe.config.Config;
import org.apache.seatunnel.shade.org.apache.commons.lang3.StringUtils;

import org.apache.seatunnel.api.configuration.ReadonlyConfig;
import org.apache.seatunnel.api.configuration.util.ConfigValidator;
import org.apache.seatunnel.api.configuration.util.OptionRule;
import org.apache.seatunnel.api.configuration.util.OptionValidationException;
import org.apache.seatunnel.api.table.factory.Factory;
import org.apache.seatunnel.api.table.factory.FactoryUtil;
import org.apache.seatunnel.api.table.factory.TableSinkFactory;
import org.apache.seatunnel.api.table.factory.TableSourceFactory;

import io.github.leonyoah.stx.proxy.model.JobConfigContext;
import io.github.leonyoah.stx.proxy.model.NodeKind;
import io.github.leonyoah.stx.proxy.service.plugin.PluginClassLoaderUtils;
import io.github.leonyoah.stx.proxy.service.plugin.PluginRuntimeService;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;

import java.net.URLClassLoader;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

import static org.apache.seatunnel.api.options.ConnectorCommonOptions.PLUGIN_NAME;

/**
 * 用引擎 {@link ConfigValidator} 对作业中每个连接器块做 OptionRule 校验（含 3.0 valueConstraints / conditionRules）。
 *
 * <p>Runs SeaTunnel {@link ConfigValidator} against each connector block (covers 3.0
 * valueConstraints / conditionRules when present).
 */
public class OptionRuleValidationService {

    private final JobConfigSupportService jobConfigSupportService;
    private final PluginRuntimeService pluginRuntimeService;

    public OptionRuleValidationService() {
        this(new JobConfigSupportService(), new PluginRuntimeService());
    }

    OptionRuleValidationService(
            JobConfigSupportService jobConfigSupportService,
            PluginRuntimeService pluginRuntimeService) {
        this.jobConfigSupportService = jobConfigSupportService;
        this.pluginRuntimeService = pluginRuntimeService;
    }

    /**
     * 校验全部 source / transform / sink；发现失败写入 errors，无法加载工厂时写入 warnings。
     *
     * <p>Validates all source/transform/sink blocks; factory load failures become warnings.
     */
    public void validate(JobConfigContext context, List<String> errors, List<String> warnings) {
        if (context == null) {
            return;
        }
        ClassLoader originalClassLoader = Thread.currentThread().getContextClassLoader();
        URLClassLoader seatunnelHomeClassLoader = null;
        try {
            seatunnelHomeClassLoader =
                    jobConfigSupportService.createSeatunnelHomeClassLoader(originalClassLoader);
            ClassLoader classLoader =
                    seatunnelHomeClassLoader != null
                            ? seatunnelHomeClassLoader
                            : originalClassLoader;
            Thread.currentThread().setContextClassLoader(classLoader);

            validateBlocks(NodeKind.SOURCE, "source", context.getSources(), errors, warnings);
            validateBlocks(
                    NodeKind.TRANSFORM, "transform", context.getTransforms(), errors, warnings);
            validateBlocks(NodeKind.SINK, "sink", context.getSinks(), errors, warnings);
        } catch (ProxyException e) {
            warnings.add("OptionRule 校验跳过：无法准备插件 ClassLoader — " + e.getMessage());
        } finally {
            Thread.currentThread().setContextClassLoader(originalClassLoader);
            PluginClassLoaderUtils.closeQuietly(seatunnelHomeClassLoader);
        }
    }

    private void validateBlocks(
            NodeKind kind,
            String pluginType,
            List<Config> blocks,
            List<String> errors,
            List<String> warnings) {
        if (blocks == null || blocks.isEmpty()) {
            return;
        }
        Map<String, Object> runtimeRequest = new LinkedHashMap<>();
        runtimeRequest.put("pluginType", pluginType);
        PluginRuntimeService.PluginExecutionContext executionContext = null;
        try {
            executionContext = pluginRuntimeService.openContext(runtimeRequest);
            Thread.currentThread().setContextClassLoader(executionContext.getClassLoader());
            for (int i = 0; i < blocks.size(); i++) {
                validateOne(kind, pluginType, i, blocks.get(i), executionContext, errors, warnings);
            }
        } catch (ProxyException e) {
            warnings.add(
                    String.format(
                            "%s OptionRule 校验跳过：%s",
                            StringUtils.capitalize(pluginType), e.getMessage()));
        } finally {
            if (executionContext != null) {
                executionContext.close();
            }
        }
    }

    private void validateOne(
            NodeKind kind,
            String pluginType,
            int index,
            Config config,
            PluginRuntimeService.PluginExecutionContext executionContext,
            List<String> errors,
            List<String> warnings) {
        ReadonlyConfig readonlyConfig = ReadonlyConfig.fromConfig(config);
        String pluginName;
        try {
            pluginName = readonlyConfig.get(PLUGIN_NAME);
        } catch (Exception e) {
            errors.add(
                    String.format(
                            "%s[%d] 缺少 plugin_name / 连接器标识",
                            StringUtils.capitalize(kind.name().toLowerCase()), index));
            return;
        }
        String label =
                String.format(
                        "%s[%d]-%s",
                        StringUtils.capitalize(kind.name().toLowerCase()), index, pluginName);
        Factory factory;
        try {
            factory = pluginRuntimeService.discoverFactory(executionContext, pluginName);
        } catch (Exception e) {
            warnings.add(label + " OptionRule 校验跳过：未找到工厂 — " + firstLine(e.getMessage()));
            return;
        }
        OptionRule optionRule;
        try {
            optionRule = resolveOptionRule(pluginType, factory);
        } catch (Exception e) {
            warnings.add(label + " OptionRule 解析失败 — " + firstLine(e.getMessage()));
            return;
        }
        if (optionRule == null) {
            warnings.add(label + " OptionRule 为空，跳过校验");
            return;
        }
        try {
            ConfigValidator.of(readonlyConfig).validate(optionRule);
        } catch (OptionValidationException e) {
            errors.add(label + " " + firstLine(e.getMessage()));
        } catch (Exception e) {
            errors.add(label + " OptionRule 校验异常 — " + firstLine(e.getMessage()));
        }
    }

    private static OptionRule resolveOptionRule(String pluginType, Factory factory) {
        switch (pluginType) {
            case "source":
                return FactoryUtil.sourceFullOptionRule((TableSourceFactory) factory);
            case "sink":
                return FactoryUtil.sinkFullOptionRule((TableSinkFactory) factory);
            case "transform":
            case "catalog":
                return factory.optionRule();
            default:
                return factory.optionRule();
        }
    }

    private static String firstLine(String message) {
        if (StringUtils.isBlank(message)) {
            return "unknown error";
        }
        int newline = message.indexOf('\n');
        return newline < 0 ? message.trim() : message.substring(0, newline).trim();
    }
}
