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

package io.github.leonyoah.stx.proxy.service.catalog;

import org.apache.seatunnel.api.configuration.ReadonlyConfig;
import org.apache.seatunnel.api.table.catalog.Catalog;
import org.apache.seatunnel.api.table.catalog.CatalogTable;
import org.apache.seatunnel.api.table.catalog.Column;
import org.apache.seatunnel.api.table.catalog.ConstraintKey;
import org.apache.seatunnel.api.table.catalog.PrimaryKey;
import org.apache.seatunnel.api.table.catalog.TablePath;
import org.apache.seatunnel.api.table.catalog.TableSchema;
import org.apache.seatunnel.api.table.factory.FactoryUtil;

import io.github.leonyoah.stx.proxy.service.plugin.PluginClassLoaderUtils;
import io.github.leonyoah.stx.proxy.service.support.ProbeExecutionUtils;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.net.URLClassLoader;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;

public class CatalogProbeService {

    public Map<String, Object> probe(Map<String, Object> request) {
        return ProbeExecutionUtils.runWithTimeout(
                request,
                "Catalog probe",
                "Check target endpoint reachability, credentials, and whether the connector jar is in SEATUNNEL_HOME/connectors while driver jars are in SEATUNNEL_HOME/plugins/*/lib or pluginJars.",
                () -> doProbe(request));
    }

    private Map<String, Object> doProbe(Map<String, Object> request) {
        String factoryIdentifier =
                ProxyRequestUtils.getRequiredString(request, "factoryIdentifier");
        String catalogName = ProxyRequestUtils.getOptionalString(request, "catalogName");
        if (catalogName == null) {
            catalogName = "stx_java_proxy_catalog";
        }
        Map<String, Object> options = ProxyRequestUtils.getMap(request, "options");
        List<String> pluginJars = ProxyRequestUtils.getStringList(request, "pluginJars");
        boolean includeDatabases = ProxyRequestUtils.getBoolean(request, "includeDatabases", true);
        String databaseName = ProxyRequestUtils.getOptionalString(request, "databaseName");
        Object rawTablePath = request.get("tablePath");

        ClassLoader parent = Thread.currentThread().getContextClassLoader();
        ClassLoader probeClassLoader = parent;
        URLClassLoader urlClassLoader = null;

        try {
            if (!pluginJars.isEmpty()) {
                urlClassLoader = PluginClassLoaderUtils.createClassLoader(pluginJars, parent);
                probeClassLoader = urlClassLoader;
            } else {
                urlClassLoader = PluginClassLoaderUtils.createClassLoaderFromSeatunnelHome(parent);
                if (urlClassLoader != null) {
                    probeClassLoader = urlClassLoader;
                }
            }

            Thread currentThread = Thread.currentThread();
            ClassLoader originalClassLoader = currentThread.getContextClassLoader();
            currentThread.setContextClassLoader(probeClassLoader);
            try {
                Optional<Catalog> optionalCatalog =
                        FactoryUtil.createOptionalCatalog(
                                catalogName,
                                ReadonlyConfig.fromMap(options),
                                probeClassLoader,
                                factoryIdentifier);
                if (!optionalCatalog.isPresent()) {
                    throw new ProxyException(
                            400, "Catalog factory not found for identifier: " + factoryIdentifier);
                }
                try (Catalog catalog = optionalCatalog.get()) {
                    catalog.open();
                    Map<String, Object> response = new LinkedHashMap<>();
                    response.put("ok", true);
                    response.put("factoryIdentifier", factoryIdentifier);
                    response.put("catalogName", catalog.name());
                    response.put("defaultDatabase", catalog.getDefaultDatabase());
                    if (includeDatabases) {
                        response.put("databases", catalog.listDatabases());
                    }
                    if (databaseName != null) {
                        response.put("tables", catalog.listTables(databaseName));
                    }
                    TablePath tablePath = resolveTablePath(rawTablePath);
                    if (tablePath != null) {
                        response.put("table", serializeCatalogTable(catalog.getTable(tablePath)));
                    }
                    return response;
                }
            } finally {
                currentThread.setContextClassLoader(originalClassLoader);
            }
        } catch (ProxyException e) {
            throw e;
        } catch (Exception e) {
            throw new ProxyException(500, "Catalog probe failed: " + e.getMessage(), e);
        } finally {
            PluginClassLoaderUtils.closeQuietly(urlClassLoader);
        }
    }

    @SuppressWarnings("unchecked")
    private TablePath resolveTablePath(Object rawTablePath) {
        if (rawTablePath == null) {
            return null;
        }
        if (rawTablePath instanceof String) {
            return TablePath.of((String) rawTablePath);
        }
        if (!(rawTablePath instanceof Map)) {
            throw new ProxyException(400, "Field 'tablePath' must be a string or object");
        }
        Map<String, Object> tablePathObject =
                new LinkedHashMap<>((Map<String, Object>) rawTablePath);
        String databaseName = ProxyRequestUtils.getOptionalString(tablePathObject, "databaseName");
        String schemaName = ProxyRequestUtils.getOptionalString(tablePathObject, "schemaName");
        String tableName = ProxyRequestUtils.getRequiredString(tablePathObject, "tableName");
        return TablePath.of(databaseName, schemaName, tableName);
    }

    private Map<String, Object> serializeCatalogTable(CatalogTable catalogTable) {
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("tablePath", catalogTable.getTablePath().getFullName());
        result.put("catalogName", catalogTable.getCatalogName());
        result.put("comment", catalogTable.getComment());
        result.put("partitionKeys", catalogTable.getPartitionKeys());
        result.put("options", catalogTable.getOptions());
        result.put("schema", serializeSchema(catalogTable.getTableSchema()));
        return result;
    }

    private Map<String, Object> serializeSchema(TableSchema tableSchema) {
        Map<String, Object> result = new LinkedHashMap<>();
        List<Map<String, Object>> columns = new ArrayList<>();
        for (Column column : tableSchema.getColumns()) {
            Map<String, Object> columnInfo = new LinkedHashMap<>();
            columnInfo.put("name", column.getName());
            columnInfo.put(
                    "dataType",
                    column.getDataType() == null ? null : column.getDataType().toString());
            columnInfo.put("columnLength", column.getColumnLength());
            columnInfo.put("scale", column.getScale());
            columnInfo.put("nullable", column.isNullable());
            columnInfo.put("defaultValue", column.getDefaultValue());
            columnInfo.put("comment", column.getComment());
            columnInfo.put("sourceType", column.getSourceType());
            columnInfo.put("sinkType", column.getSinkType());
            columnInfo.put("options", column.getOptions());
            columns.add(columnInfo);
        }
        result.put("columns", columns);

        PrimaryKey primaryKey = tableSchema.getPrimaryKey();
        if (primaryKey == null) {
            result.put("primaryKey", null);
        } else {
            Map<String, Object> primaryKeyInfo = new LinkedHashMap<>();
            primaryKeyInfo.put("name", primaryKey.getPrimaryKey());
            primaryKeyInfo.put("columnNames", primaryKey.getColumnNames());
            primaryKeyInfo.put("enableAutoId", primaryKey.getEnableAutoId());
            result.put("primaryKey", primaryKeyInfo);
        }

        List<Map<String, Object>> constraints = new ArrayList<>();
        if (tableSchema.getConstraintKeys() != null) {
            for (ConstraintKey constraintKey : tableSchema.getConstraintKeys()) {
                Map<String, Object> constraintInfo = new LinkedHashMap<>();
                constraintInfo.put("constraintType", constraintKey.getConstraintType().name());
                constraintInfo.put("constraintName", constraintKey.getConstraintName());
                List<Map<String, Object>> columnsInfo = new ArrayList<>();
                if (constraintKey.getColumnNames() != null) {
                    for (ConstraintKey.ConstraintKeyColumn column :
                            constraintKey.getColumnNames()) {
                        Map<String, Object> columnInfo = new LinkedHashMap<>();
                        columnInfo.put("columnName", column.getColumnName());
                        columnInfo.put(
                                "sortType",
                                column.getSortType() == null ? null : column.getSortType().name());
                        columnsInfo.add(columnInfo);
                    }
                }
                constraintInfo.put("columns", columnsInfo);
                constraints.add(constraintInfo);
            }
        }
        result.put("constraintKeys", constraints);
        return result;
    }
}
