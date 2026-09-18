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
import org.apache.seatunnel.api.sink.DataSaveMode;
import org.apache.seatunnel.api.sink.SaveModeHandler;
import org.apache.seatunnel.api.sink.SchemaSaveMode;
import org.apache.seatunnel.api.sink.SeaTunnelSink;
import org.apache.seatunnel.api.sink.SupportSaveMode;
import org.apache.seatunnel.api.table.catalog.Catalog;
import org.apache.seatunnel.api.table.catalog.CatalogTable;
import org.apache.seatunnel.api.table.catalog.InfoPreviewResult;
import org.apache.seatunnel.api.table.catalog.PreviewResult;
import org.apache.seatunnel.api.table.catalog.SQLPreviewResult;
import org.apache.seatunnel.api.table.catalog.TablePath;
import org.apache.seatunnel.api.table.factory.FactoryUtil;
import org.apache.seatunnel.plugin.discovery.seatunnel.SeaTunnelSinkPluginDiscovery;

import io.github.leonyoah.stx.proxy.model.JobConfigContext;
import io.github.leonyoah.stx.proxy.model.NodeKind;
import io.github.leonyoah.stx.proxy.service.plugin.PluginClassLoaderUtils;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.io.IOException;
import java.net.URLClassLoader;
import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.Set;
import java.util.function.Function;

import static org.apache.seatunnel.api.options.ConnectorCommonOptions.PLUGIN_NAME;

public class SinkSaveModePreviewService {

    private final JobConfigSupportService jobConfigSupportService;

    public SinkSaveModePreviewService() {
        this(new JobConfigSupportService());
    }

    SinkSaveModePreviewService(JobConfigSupportService jobConfigSupportService) {
        this.jobConfigSupportService = jobConfigSupportService;
    }

    public Map<String, Object> preview(Map<String, Object> request) {
        JobConfigContext context = jobConfigSupportService.parseJobContext(request);
        if (context.getSinks().isEmpty()) {
            throw new ProxyException(400, "Save mode preview requires at least one sink");
        }
        int sinkIndex = resolveSinkIndex(request, context);
        return preview(context, request, sinkIndex);
    }

    Map<String, Object> preview(
            JobConfigContext context, Map<String, Object> request, int sinkIndex) {
        Config sinkConfig = context.getSinks().get(sinkIndex);
        ReadonlyConfig sinkReadonlyConfig = ReadonlyConfig.fromConfig(sinkConfig);
        String connector = sinkReadonlyConfig.get(PLUGIN_NAME);
        List<String> warnings = new ArrayList<>(context.getWarnings());

        ClassLoader originalClassLoader = Thread.currentThread().getContextClassLoader();
        URLClassLoader pluginClassLoader = null;
        try {
            pluginClassLoader = buildPreviewClassLoader(request, originalClassLoader);
            ClassLoader effectiveClassLoader =
                    pluginClassLoader != null ? pluginClassLoader : originalClassLoader;
            if (pluginClassLoader != null) {
                Thread.currentThread().setContextClassLoader(pluginClassLoader);
            }

            Map<String, List<CatalogTable>> datasetCatalogTables =
                    resolveDatasetCatalogTables(context, effectiveClassLoader);
            List<CatalogTable> inputCatalogTables =
                    jobConfigSupportService.collectInputCatalogTables(
                            sinkReadonlyConfig, datasetCatalogTables);
            if (inputCatalogTables.isEmpty()) {
                throw new ProxyException(
                        400,
                        String.format(
                                "Sink[%d]-%s has no upstream CatalogTable available for save mode preview",
                                sinkIndex, connector));
            }

            List<Map<String, Object>> previewEntries = new ArrayList<>();
            Set<String> completeness = new LinkedHashSet<>();
            boolean supported = false;
            for (CatalogTable inputCatalogTable : inputCatalogTables) {
                Map<String, Object> entry =
                        previewSingleTable(
                                sinkReadonlyConfig,
                                connector,
                                inputCatalogTable,
                                effectiveClassLoader,
                                warnings,
                                completeness);
                previewEntries.add(entry);
                supported |= Boolean.TRUE.equals(entry.get("supported"));
            }

            Map<String, Object> result = new LinkedHashMap<>();
            result.put("ok", true);
            result.put("connector", connector);
            result.put("sinkIndex", sinkIndex);
            result.put("supported", supported);
            result.put("completeness", summarizeCompleteness(completeness, supported));
            if (previewEntries.size() == 1) {
                result.putAll(previewEntries.get(0));
            } else {
                result.put("tables", previewEntries);
            }
            result.put("warnings", warnings);
            return result;
        } finally {
            Thread.currentThread().setContextClassLoader(originalClassLoader);
            PluginClassLoaderUtils.closeQuietly(pluginClassLoader);
        }
    }

    Map<String, Map<String, Object>> toVertexTablePreviews(
            Map<String, Object> previewResult, List<String> fallbackTablePaths) {
        Map<String, Map<String, Object>> normalized = new LinkedHashMap<>();
        if (previewResult == null || previewResult.isEmpty()) {
            return normalized;
        }
        List<Map<String, Object>> tables = getMapList(previewResult, "tables");
        if (!tables.isEmpty()) {
            for (Map<String, Object> table : tables) {
                String tablePath = ProxyRequestUtils.getOptionalString(table, "tablePath");
                if (StringUtils.isBlank(tablePath)) {
                    continue;
                }
                Map<String, Object> entry = copyTablePreview(table);
                entry.put("warnings", ProxyRequestUtils.getStringList(previewResult, "warnings"));
                normalized.put(tablePath, entry);
            }
            return normalized;
        }

        String tablePath = ProxyRequestUtils.getOptionalString(previewResult, "tablePath");
        if (StringUtils.isBlank(tablePath)
                && fallbackTablePaths != null
                && !fallbackTablePaths.isEmpty()) {
            tablePath = fallbackTablePaths.get(0);
        }
        if (StringUtils.isBlank(tablePath)) {
            tablePath = "__default__";
        }
        Map<String, Object> entry = copyTablePreview(previewResult);
        entry.put("warnings", ProxyRequestUtils.getStringList(previewResult, "warnings"));
        normalized.put(tablePath, entry);
        return normalized;
    }

    private Map<String, Object> copyTablePreview(Map<String, Object> source) {
        Map<String, Object> entry = new LinkedHashMap<>();
        copyIfPresent(source, entry, "tablePath");
        copyIfPresent(source, entry, "supported");
        copyIfPresent(source, entry, "completeness");
        copyIfPresent(source, entry, "schemaSaveMode");
        copyIfPresent(source, entry, "dataSaveMode");
        if (source.containsKey("actions")) {
            entry.put("actions", getMapList(source, "actions"));
        }
        return entry;
    }

    private void copyIfPresent(Map<String, Object> source, Map<String, Object> target, String key) {
        if (source.containsKey(key)) {
            target.put(key, source.get(key));
        }
    }

    @SuppressWarnings("unchecked")
    private List<Map<String, Object>> getMapList(Map<String, Object> source, String key) {
        if (source == null || !source.containsKey(key) || source.get(key) == null) {
            return Collections.emptyList();
        }
        Object value = source.get(key);
        if (!(value instanceof List)) {
            throw new ProxyException(400, "Field '" + key + "' must be an array");
        }
        List<Map<String, Object>> result = new ArrayList<>();
        for (Object item : (List<Object>) value) {
            if (!(item instanceof Map)) {
                throw new ProxyException(400, "Field '" + key + "' must contain objects");
            }
            result.add(new LinkedHashMap<>((Map<String, Object>) item));
        }
        return result;
    }

    private int resolveSinkIndex(Map<String, Object> request, JobConfigContext context) {
        String sinkNodeId = ProxyRequestUtils.getOptionalString(request, "sinkNodeId");
        if (StringUtils.isNotBlank(sinkNodeId)) {
            for (int i = 0; i < context.getSinks().size(); i++) {
                if (sinkNodeId.equals(jobConfigSupportService.nodeId(NodeKind.SINK, i))) {
                    return i;
                }
            }
            throw new ProxyException(400, "Unknown sinkNodeId: " + sinkNodeId);
        }
        int sinkIndex = (int) ProxyRequestUtils.getLong(request, "sinkIndex", 0);
        if (sinkIndex < 0 || sinkIndex >= context.getSinks().size()) {
            throw new ProxyException(
                    400,
                    String.format(
                            "sinkIndex out of range: %d (available sinks=%d)",
                            sinkIndex, context.getSinks().size()));
        }
        return sinkIndex;
    }

    private URLClassLoader buildPreviewClassLoader(
            Map<String, Object> request, ClassLoader parent) {
        List<String> pluginJars = ProxyRequestUtils.getStringList(request, "pluginJars");
        try {
            if (!pluginJars.isEmpty()) {
                return PluginClassLoaderUtils.createClassLoader(pluginJars, parent);
            }
            return PluginClassLoaderUtils.createClassLoaderFromSeatunnelHome(parent);
        } catch (IOException e) {
            throw new ProxyException(
                    500, "Failed to build preview classloader: " + e.getMessage(), e);
        }
    }

    private Map<String, List<CatalogTable>> resolveDatasetCatalogTables(
            JobConfigContext context, ClassLoader classLoader) {
        Map<String, List<CatalogTable>> datasetCatalogTables = new LinkedHashMap<>();
        for (Config sourceConfig : context.getSources()) {
            ReadonlyConfig readonlyConfig = ReadonlyConfig.fromConfig(sourceConfig);
            String output = jobConfigSupportService.getOutputDataset(readonlyConfig);
            datasetCatalogTables.put(
                    output,
                    jobConfigSupportService.resolveSourceCatalogTables(
                            readonlyConfig, classLoader));
        }
        for (Config transformConfig : context.getTransforms()) {
            ReadonlyConfig readonlyConfig = ReadonlyConfig.fromConfig(transformConfig);
            String output = jobConfigSupportService.getOutputDataset(readonlyConfig);
            List<CatalogTable> inputCatalogTables =
                    jobConfigSupportService.collectInputCatalogTables(
                            readonlyConfig, datasetCatalogTables);
            datasetCatalogTables.put(
                    output,
                    jobConfigSupportService.resolveTransformCatalogTables(
                            readonlyConfig, inputCatalogTables, classLoader));
        }
        return datasetCatalogTables;
    }

    @SuppressWarnings({"rawtypes", "unchecked"})
    private Map<String, Object> previewSingleTable(
            ReadonlyConfig sinkReadonlyConfig,
            String connector,
            CatalogTable inputCatalogTable,
            ClassLoader classLoader,
            List<String> warnings,
            Set<String> completenessRecorder) {
        String factoryId = sinkReadonlyConfig.get(PLUGIN_NAME);
        Function<org.apache.seatunnel.api.common.PluginIdentifier, SeaTunnelSink>
                fallbackCreateSink =
                        pluginIdentifier ->
                                new SeaTunnelSinkPluginDiscovery()
                                        .createPluginInstance(pluginIdentifier);
        SeaTunnelSink<?, ?, ?, ?> sink =
                FactoryUtil.createAndPrepareSink(
                        inputCatalogTable,
                        sinkReadonlyConfig,
                        classLoader,
                        factoryId,
                        fallbackCreateSink,
                        null);
        Map<String, Object> entry = new LinkedHashMap<>();
        CatalogTable writeCatalogTable = sink.getWriteCatalogTable().orElse(inputCatalogTable);
        String tablePath =
                writeCatalogTable != null && writeCatalogTable.getTablePath() != null
                        ? writeCatalogTable.getTablePath().getFullName()
                        : null;
        entry.put("tablePath", tablePath);

        if (!(sink instanceof SupportSaveMode)) {
            entry.put("supported", false);
            entry.put("completeness", "UNSUPPORTED");
            entry.put("actions", Collections.emptyList());
            warnings.add(connector + " sink does not implement SupportSaveMode");
            completenessRecorder.add("UNSUPPORTED");
            return entry;
        }

        Optional<SaveModeHandler> handlerOptional = ((SupportSaveMode) sink).getSaveModeHandler();
        if (!handlerOptional.isPresent()) {
            entry.put("supported", false);
            entry.put("completeness", "UNSUPPORTED");
            entry.put("actions", Collections.emptyList());
            warnings.add(connector + " sink returned empty SaveModeHandler");
            completenessRecorder.add("UNSUPPORTED");
            return entry;
        }

        SaveModeHandler handler = handlerOptional.get();
        try {
            handler.open();
            SchemaSaveMode schemaSaveMode = handler.getSchemaSaveMode();
            DataSaveMode dataSaveMode = handler.getDataSaveMode();
            Catalog catalog = handler.getHandleCatalog();
            TablePath handleTablePath = handler.getHandleTablePath();
            entry.put("supported", true);
            entry.put("schemaSaveMode", schemaSaveMode == null ? null : schemaSaveMode.name());
            entry.put("dataSaveMode", dataSaveMode == null ? null : dataSaveMode.name());
            entry.put(
                    "tablePath",
                    handleTablePath == null ? tablePath : handleTablePath.getFullName());

            ActionPlan actionPlan =
                    ActionPlan.plan(
                            connector,
                            sinkReadonlyConfig,
                            catalog,
                            handleTablePath,
                            writeCatalogTable,
                            schemaSaveMode,
                            dataSaveMode,
                            warnings);
            List<Map<String, Object>> actions = new ArrayList<>();
            int sqlCount = 0;
            int infoCount = 0;
            int unsupportedCount = 0;
            for (PlannedAction plannedAction : actionPlan.actions) {
                Map<String, Object> actionMap = new LinkedHashMap<>();
                actionMap.put("phase", plannedAction.phase);
                actionMap.put("actionType", plannedAction.actionType);
                if (plannedAction.customSql != null) {
                    actionMap.put("resultType", "SQL");
                    actionMap.put("content", plannedAction.customSql);
                    actionMap.put("native", false);
                    sqlCount++;
                    actions.add(actionMap);
                    continue;
                }
                PreviewResult previewResult =
                        catalog.previewAction(
                                plannedAction.catalogActionType,
                                handleTablePath,
                                plannedAction.includeCatalogTable
                                        ? Optional.ofNullable(writeCatalogTable)
                                        : Optional.empty());
                if (previewResult instanceof SQLPreviewResult) {
                    actionMap.put("resultType", "SQL");
                    actionMap.put("content", ((SQLPreviewResult) previewResult).getSql());
                    actionMap.put("native", true);
                    sqlCount++;
                } else if (previewResult instanceof InfoPreviewResult) {
                    actionMap.put("resultType", "INFO");
                    actionMap.put("content", ((InfoPreviewResult) previewResult).getInfo());
                    actionMap.put("native", true);
                    infoCount++;
                } else {
                    actionMap.put("resultType", "UNKNOWN");
                    actionMap.put("content", String.valueOf(previewResult));
                    actionMap.put("native", true);
                    infoCount++;
                }
                actions.add(actionMap);
            }
            if (actionPlan.actions.isEmpty()) {
                entry.put("actions", Collections.emptyList());
                entry.put("completeness", "INFO_ONLY");
                completenessRecorder.add("INFO_ONLY");
                if (!actionPlan.warnings.isEmpty()) {
                    warnings.addAll(actionPlan.warnings);
                }
                return entry;
            }
            entry.put("actions", actions);
            if (!actionPlan.warnings.isEmpty()) {
                warnings.addAll(actionPlan.warnings);
            }
            String completeness = summarizeCompleteness(sqlCount, infoCount, unsupportedCount);
            entry.put("completeness", completeness);
            completenessRecorder.add(completeness);
            return entry;
        } catch (UnsupportedOperationException e) {
            entry.put("supported", false);
            entry.put("completeness", "UNSUPPORTED");
            entry.put("actions", Collections.emptyList());
            warnings.add(connector + " save mode preview unsupported: " + e.getMessage());
            completenessRecorder.add("UNSUPPORTED");
            return entry;
        } catch (Exception e) {
            entry.put("supported", false);
            entry.put("completeness", "UNSUPPORTED");
            entry.put("actions", Collections.emptyList());
            warnings.add(connector + " save mode preview failed: " + e.getMessage());
            completenessRecorder.add("UNSUPPORTED");
            return entry;
        } finally {
            try {
                handler.close();
            } catch (Exception ignored) {
                // no-op
            }
        }
    }

    private String summarizeCompleteness(Set<String> completeness, boolean supported) {
        if (!supported) {
            return "UNSUPPORTED";
        }
        if (completeness.contains("PARTIAL")) {
            return "PARTIAL";
        }
        if (completeness.contains("INFO_ONLY") && completeness.contains("FULL_SQL")) {
            return "PARTIAL";
        }
        if (completeness.contains("FULL_SQL")) {
            return "FULL_SQL";
        }
        if (completeness.contains("INFO_ONLY")) {
            return "INFO_ONLY";
        }
        return "UNSUPPORTED";
    }

    private String summarizeCompleteness(int sqlCount, int infoCount, int unsupportedCount) {
        if (unsupportedCount > 0) {
            return sqlCount > 0 || infoCount > 0 ? "PARTIAL" : "UNSUPPORTED";
        }
        if (sqlCount > 0 && infoCount == 0) {
            return "FULL_SQL";
        }
        if (sqlCount > 0) {
            return "PARTIAL";
        }
        if (infoCount > 0) {
            return "INFO_ONLY";
        }
        return "UNSUPPORTED";
    }

    private static final class ActionPlan {
        private final List<PlannedAction> actions;
        private final List<String> warnings;

        private ActionPlan(List<PlannedAction> actions, List<String> warnings) {
            this.actions = actions;
            this.warnings = warnings;
        }

        private static ActionPlan plan(
                String connector,
                ReadonlyConfig sinkConfig,
                Catalog catalog,
                TablePath tablePath,
                CatalogTable catalogTable,
                SchemaSaveMode schemaSaveMode,
                DataSaveMode dataSaveMode,
                List<String> sharedWarnings) {
            List<PlannedAction> actions = new ArrayList<>();
            List<String> warnings = new ArrayList<>();
            boolean databaseExists = false;
            boolean tableExists = false;
            boolean hasState = false;
            if (catalog != null && tablePath != null) {
                try {
                    databaseExists = catalog.databaseExists(tablePath.getDatabaseName());
                    tableExists = catalog.tableExists(tablePath);
                    hasState = true;
                } catch (Exception e) {
                    warnings.add(
                            "Existence checks failed, preview will fall back to approximate actions: "
                                    + e.getMessage());
                }
            }
            boolean willCreateTable = false;
            if (schemaSaveMode == SchemaSaveMode.RECREATE_SCHEMA) {
                if (!hasState || tableExists) {
                    actions.add(PlannedAction.schema(Catalog.ActionType.DROP_TABLE));
                }
                if (!hasState || !databaseExists) {
                    actions.add(PlannedAction.schema(Catalog.ActionType.CREATE_DATABASE));
                }
                actions.add(PlannedAction.schema(Catalog.ActionType.CREATE_TABLE, true));
                willCreateTable = true;
            } else if (schemaSaveMode == SchemaSaveMode.CREATE_SCHEMA_WHEN_NOT_EXIST) {
                if (!hasState || !databaseExists) {
                    actions.add(PlannedAction.schema(Catalog.ActionType.CREATE_DATABASE));
                }
                if (!hasState || !tableExists) {
                    actions.add(PlannedAction.schema(Catalog.ActionType.CREATE_TABLE, true));
                    willCreateTable = true;
                }
            } else if (schemaSaveMode == SchemaSaveMode.ERROR_WHEN_SCHEMA_NOT_EXIST) {
                warnings.add(
                        "Schema save mode performs existence checks only and may not produce preview SQL.");
            }

            if (dataSaveMode == DataSaveMode.DROP_DATA) {
                if (!willCreateTable) {
                    actions.add(PlannedAction.data(Catalog.ActionType.TRUNCATE_TABLE));
                }
            } else if (dataSaveMode == DataSaveMode.CUSTOM_PROCESSING) {
                Object customSqlValue = sinkConfig.getSourceMap().get("custom_sql");
                String customSql = customSqlValue == null ? null : String.valueOf(customSqlValue);
                if (StringUtils.isNotBlank(customSql)) {
                    actions.add(PlannedAction.custom(customSql));
                } else {
                    warnings.add("Data save mode is CUSTOM_PROCESSING but custom_sql is empty.");
                }
            } else if (dataSaveMode == DataSaveMode.ERROR_WHEN_DATA_EXISTS) {
                warnings.add(
                        "Data save mode checks data existence only and may not produce preview SQL.");
            }

            if ("Hive".equalsIgnoreCase(connector)) {
                warnings.add(
                        "Hive save mode preview may be partial; if save_mode_create_template is absent, creation may fall back to Metastore API.");
            }
            if ("MaxCompute".equalsIgnoreCase(connector)) {
                warnings.add(
                        "MaxCompute save mode preview may be partial; partition-related actions may execute through SDK instead of SQL.");
            }
            sharedWarnings.addAll(warnings);
            return new ActionPlan(actions, warnings);
        }
    }

    private static final class PlannedAction {
        private final String phase;
        private final String actionType;
        private final Catalog.ActionType catalogActionType;
        private final boolean includeCatalogTable;
        private final String customSql;

        private PlannedAction(
                String phase,
                String actionType,
                Catalog.ActionType catalogActionType,
                boolean includeCatalogTable,
                String customSql) {
            this.phase = phase;
            this.actionType = actionType;
            this.catalogActionType = catalogActionType;
            this.includeCatalogTable = includeCatalogTable;
            this.customSql = customSql;
        }

        private static PlannedAction schema(Catalog.ActionType actionType) {
            return new PlannedAction("SCHEMA", actionType.name(), actionType, false, null);
        }

        private static PlannedAction schema(
                Catalog.ActionType actionType, boolean includeCatalogTable) {
            return new PlannedAction(
                    "SCHEMA", actionType.name(), actionType, includeCatalogTable, null);
        }

        private static PlannedAction data(Catalog.ActionType actionType) {
            return new PlannedAction("DATA", actionType.name(), actionType, false, null);
        }

        private static PlannedAction custom(String sql) {
            return new PlannedAction("DATA", "CUSTOM_SQL", null, false, sql);
        }
    }
}
