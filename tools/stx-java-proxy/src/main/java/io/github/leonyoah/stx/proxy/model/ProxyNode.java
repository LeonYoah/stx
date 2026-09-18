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

public class ProxyNode {

    private final String nodeId;
    private final NodeKind kind;
    private final String pluginName;
    private final int configIndex;
    private final List<String> inputDatasets;
    private final String outputDataset;
    private final List<String> tablePaths;
    private final Map<String, List<String>> tableColumns;
    private final Map<String, Map<String, Object>> tableSchemas;

    public ProxyNode(
            String nodeId,
            NodeKind kind,
            String pluginName,
            int configIndex,
            List<String> inputDatasets,
            String outputDataset,
            List<String> tablePaths,
            Map<String, List<String>> tableColumns,
            Map<String, Map<String, Object>> tableSchemas) {
        this.nodeId = nodeId;
        this.kind = kind;
        this.pluginName = pluginName;
        this.configIndex = configIndex;
        this.inputDatasets =
                inputDatasets == null
                        ? Collections.emptyList()
                        : Collections.unmodifiableList(inputDatasets);
        this.outputDataset = outputDataset;
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
    }

    public String getNodeId() {
        return nodeId;
    }

    public NodeKind getKind() {
        return kind;
    }

    public String getPluginName() {
        return pluginName;
    }

    public int getConfigIndex() {
        return configIndex;
    }

    public List<String> getInputDatasets() {
        return inputDatasets;
    }

    public String getOutputDataset() {
        return outputDataset;
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
}
