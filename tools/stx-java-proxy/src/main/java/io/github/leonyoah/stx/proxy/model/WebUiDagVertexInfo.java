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
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

public class WebUiDagVertexInfo {

    private final Integer vertexId;
    private final String type;
    private final String connectorType;
    private final List<String> tablePaths;
    private final Map<String, List<String>> tableColumns;
    private final Map<String, Map<String, Object>> tableSchemas;
    private final Map<String, Map<String, Object>> saveModePreviews;
    private final List<String> saveModeWarnings;

    public WebUiDagVertexInfo(
            Integer vertexId,
            String type,
            String connectorType,
            List<String> tablePaths,
            Map<String, List<String>> tableColumns,
            Map<String, Map<String, Object>> tableSchemas,
            Map<String, Map<String, Object>> saveModePreviews,
            List<String> saveModeWarnings) {
        this.vertexId = vertexId;
        this.type = type;
        this.connectorType = connectorType;
        this.tablePaths =
                tablePaths == null
                        ? Collections.emptyList()
                        : Collections.unmodifiableList(tablePaths);
        this.tableColumns =
                tableColumns == null
                        ? Collections.emptyMap()
                        : Collections.unmodifiableMap(new LinkedHashMap<>(tableColumns));
        this.tableSchemas =
                tableSchemas == null
                        ? Collections.emptyMap()
                        : Collections.unmodifiableMap(new LinkedHashMap<>(tableSchemas));
        this.saveModePreviews =
                saveModePreviews == null
                        ? Collections.emptyMap()
                        : Collections.unmodifiableMap(new LinkedHashMap<>(saveModePreviews));
        this.saveModeWarnings =
                saveModeWarnings == null
                        ? Collections.emptyList()
                        : Collections.unmodifiableList(saveModeWarnings);
    }

    public Integer getVertexId() {
        return vertexId;
    }

    public String getType() {
        return type;
    }

    public String getConnectorType() {
        return connectorType;
    }

    public List<String> getTablePaths() {
        return tablePaths;
    }

    public Map<String, List<String>> getTableColumns() {
        return tableColumns;
    }

    public Map<String, Map<String, Object>> getTableSchemas() {
        return tableSchemas;
    }

    public Map<String, Map<String, Object>> getSaveModePreviews() {
        return saveModePreviews;
    }

    public List<String> getSaveModeWarnings() {
        return saveModeWarnings;
    }
}
