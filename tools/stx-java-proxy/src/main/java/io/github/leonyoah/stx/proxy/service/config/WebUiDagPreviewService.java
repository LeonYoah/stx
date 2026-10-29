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

import org.apache.seatunnel.shade.org.apache.commons.lang3.StringUtils;

import io.github.leonyoah.stx.proxy.model.DatasetDag;
import io.github.leonyoah.stx.proxy.model.JobConfigContext;
import io.github.leonyoah.stx.proxy.model.NodeKind;
import io.github.leonyoah.stx.proxy.model.ProxyEdge;
import io.github.leonyoah.stx.proxy.model.ProxyNode;
import io.github.leonyoah.stx.proxy.model.WebUiDagEdge;
import io.github.leonyoah.stx.proxy.model.WebUiDagPreviewResult;
import io.github.leonyoah.stx.proxy.model.WebUiDagVertexInfo;
import io.github.leonyoah.stx.proxy.model.WebUiJobDag;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Comparator;
import java.util.Deque;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

public class WebUiDagPreviewService {

    private static final String PREVIEW_JOB_ID = "preview";

    private final JobConfigSupportService jobConfigSupportService;
    private final SinkSaveModePreviewService sinkSaveModePreviewService;

    public WebUiDagPreviewService() {
        this(new JobConfigSupportService(), null);
    }

    WebUiDagPreviewService(JobConfigSupportService jobConfigSupportService) {
        this(jobConfigSupportService, null);
    }

    WebUiDagPreviewService(
            JobConfigSupportService jobConfigSupportService,
            SinkSaveModePreviewService sinkSaveModePreviewService) {
        this.jobConfigSupportService = jobConfigSupportService;
        this.sinkSaveModePreviewService =
                sinkSaveModePreviewService == null
                        ? new SinkSaveModePreviewService(jobConfigSupportService)
                        : sinkSaveModePreviewService;
    }

    public WebUiDagPreviewResult preview(Map<String, Object> request) {
        JobConfigContext context = jobConfigSupportService.parseJobContext(request);
        WebUiJobDag jobDag = toWebUiJobDag(context, request);
        return new WebUiDagPreviewResult(
                PREVIEW_JOB_ID,
                "Config Preview",
                "CREATED",
                "",
                "",
                "",
                jobDag,
                buildEmptyMetrics(),
                Collections.emptyList(),
                context.isSimpleGraph(),
                context.getWarnings());
    }

    private WebUiJobDag toWebUiJobDag(JobConfigContext context, Map<String, Object> request) {
        DatasetDag graph = context.getGraph();
        List<ProxyNode> orderedNodes = topologicalSort(graph);
        Map<String, Integer> nodeIds = new LinkedHashMap<>();
        Map<Integer, WebUiDagVertexInfo> vertexInfoMap = new LinkedHashMap<>();
        for (int i = 0; i < orderedNodes.size(); i++) {
            ProxyNode node = orderedNodes.get(i);
            int vertexId = i + 1;
            nodeIds.put(node.getNodeId(), vertexId);
            SaveModePreviewAttachment saveModePreviewAttachment =
                    resolveSaveModePreviewAttachment(context, request, node);
            vertexInfoMap.put(
                    vertexId,
                    new WebUiDagVertexInfo(
                            vertexId,
                            mapType(node.getKind()),
                            buildConnectorType(node),
                            buildTablePaths(node),
                            node.getTableColumns(),
                            node.getTableSchemas(),
                            saveModePreviewAttachment.previews,
                            saveModePreviewAttachment.warnings));
        }

        List<WebUiDagEdge> edges = new ArrayList<>();
        for (ProxyEdge edge : graph.getEdges()) {
            Integer from = nodeIds.get(edge.getFromNodeId());
            Integer to = nodeIds.get(edge.getToNodeId());
            if (from == null || to == null) {
                continue;
            }
            edges.add(new WebUiDagEdge(from, to));
        }
        Map<Integer, List<WebUiDagEdge>> pipelineEdges = new LinkedHashMap<>();
        pipelineEdges.put(0, Collections.unmodifiableList(edges));
        return new WebUiJobDag(
                PREVIEW_JOB_ID, pipelineEdges, vertexInfoMap, Collections.emptyMap());
    }

    private SaveModePreviewAttachment resolveSaveModePreviewAttachment(
            JobConfigContext context, Map<String, Object> request, ProxyNode node) {
        if (node.getKind() != NodeKind.SINK) {
            return SaveModePreviewAttachment.empty();
        }
        try {
            Map<String, Object> previewResult =
                    sinkSaveModePreviewService.preview(context, request, node.getConfigIndex());
            return new SaveModePreviewAttachment(
                    sinkSaveModePreviewService.toVertexTablePreviews(
                            previewResult, buildTablePaths(node)),
                    ProxyRequestUtils.getStringList(previewResult, "warnings"));
        } catch (Exception e) {
            return new SaveModePreviewAttachment(
                    Collections.emptyMap(),
                    Collections.singletonList(
                            String.format(
                                    "SaveMode preview skipped for %s: %s",
                                    buildConnectorType(node), e.getMessage())));
        }
    }

    private static class SaveModePreviewAttachment {
        private final Map<String, Map<String, Object>> previews;
        private final List<String> warnings;

        private SaveModePreviewAttachment(
                Map<String, Map<String, Object>> previews, List<String> warnings) {
            this.previews = previews == null ? Collections.emptyMap() : previews;
            this.warnings = warnings == null ? Collections.emptyList() : warnings;
        }

        private static SaveModePreviewAttachment empty() {
            return new SaveModePreviewAttachment(Collections.emptyMap(), Collections.emptyList());
        }
    }

    private List<ProxyNode> topologicalSort(DatasetDag graph) {
        Map<String, ProxyNode> nodeMap = new LinkedHashMap<>();
        Map<String, Integer> originalOrder = new LinkedHashMap<>();
        for (int i = 0; i < graph.getNodes().size(); i++) {
            ProxyNode node = graph.getNodes().get(i);
            nodeMap.put(node.getNodeId(), node);
            originalOrder.put(node.getNodeId(), i);
        }

        Map<String, Integer> indegree = new LinkedHashMap<>();
        Map<String, Set<String>> outgoing = new LinkedHashMap<>();
        for (ProxyNode node : graph.getNodes()) {
            indegree.put(node.getNodeId(), 0);
            outgoing.put(node.getNodeId(), new LinkedHashSet<>());
        }

        for (ProxyEdge edge : graph.getEdges()) {
            if (!nodeMap.containsKey(edge.getFromNodeId())
                    || !nodeMap.containsKey(edge.getToNodeId())) {
                continue;
            }
            if (outgoing.get(edge.getFromNodeId()).add(edge.getToNodeId())) {
                indegree.put(edge.getToNodeId(), indegree.get(edge.getToNodeId()) + 1);
            }
        }

        List<ProxyNode> zeroNodes = new ArrayList<>();
        for (ProxyNode node : graph.getNodes()) {
            if (indegree.get(node.getNodeId()) == 0) {
                zeroNodes.add(node);
            }
        }
        zeroNodes.sort(nodeComparator(originalOrder));

        Deque<ProxyNode> queue = new ArrayDeque<>(zeroNodes);
        List<ProxyNode> ordered = new ArrayList<>();
        while (!queue.isEmpty()) {
            ProxyNode current = queue.removeFirst();
            ordered.add(current);
            List<ProxyNode> nextNodes = new ArrayList<>();
            for (String nextNodeId : outgoing.get(current.getNodeId())) {
                int nextIndegree = indegree.get(nextNodeId) - 1;
                indegree.put(nextNodeId, nextIndegree);
                if (nextIndegree == 0) {
                    nextNodes.add(nodeMap.get(nextNodeId));
                }
            }
            nextNodes.sort(nodeComparator(originalOrder));
            for (ProxyNode node : nextNodes) {
                queue.addLast(node);
            }
        }

        if (ordered.size() == graph.getNodes().size()) {
            return ordered;
        }

        List<ProxyNode> fallback = new ArrayList<>(graph.getNodes());
        fallback.sort(nodeComparator(originalOrder));
        return fallback;
    }

    private Comparator<ProxyNode> nodeComparator(Map<String, Integer> originalOrder) {
        return Comparator.comparing((ProxyNode node) -> node.getKind().ordinal())
                .thenComparingInt(
                        node -> originalOrder.getOrDefault(node.getNodeId(), Integer.MAX_VALUE));
    }

    private String mapType(NodeKind kind) {
        switch (kind) {
            case SOURCE:
                return "source";
            case SINK:
                return "sink";
            case TRANSFORM:
            default:
                return "transform";
        }
    }

    private String buildConnectorType(ProxyNode node) {
        String prefix = StringUtils.capitalize(mapType(node.getKind()));
        return String.format("%s[%d]-%s", prefix, node.getConfigIndex(), node.getPluginName());
    }

    private List<String> buildTablePaths(ProxyNode node) {
        if (node.getTablePaths() != null && !node.getTablePaths().isEmpty()) {
            return node.getTablePaths();
        }
        List<String> candidates = new ArrayList<>();
        if (node.getKind() == NodeKind.SOURCE) {
            addIfNotBlank(candidates, node.getOutputDataset());
        } else if (node.getKind() == NodeKind.TRANSFORM) {
            addIfNotBlank(candidates, node.getOutputDataset());
            candidates.addAll(node.getInputDatasets());
        } else {
            candidates.addAll(node.getInputDatasets());
        }
        if (candidates.isEmpty()) {
            return Collections.singletonList("default");
        }
        List<String> deduped = new ArrayList<>();
        Set<String> seen = new LinkedHashSet<>();
        for (String value : candidates) {
            if (StringUtils.isBlank(value)) {
                continue;
            }
            if (seen.add(value)) {
                deduped.add(value);
            }
        }
        return deduped.isEmpty() ? Collections.singletonList("default") : deduped;
    }

    private void addIfNotBlank(List<String> values, String value) {
        if (StringUtils.isNotBlank(value)) {
            values.add(value);
        }
    }

    private Map<String, Object> buildEmptyMetrics() {
        Map<String, Object> metrics = new LinkedHashMap<>();
        metrics.put("SinkWriteCount", "0");
        metrics.put("SinkWriteBytesPerSeconds", "0");
        metrics.put("SinkWriteQPS", "0");
        metrics.put("SourceReceivedBytes", "0");
        metrics.put("SourceReceivedBytesPerSeconds", "0");
        metrics.put("SourceReceivedCount", "0");
        metrics.put("SourceReceivedQPS", "0");
        metrics.put("SinkWriteBytes", "0");
        metrics.put("TableSourceReceivedBytes", Collections.emptyMap());
        metrics.put("TableSourceReceivedCount", Collections.emptyMap());
        metrics.put("TableSourceReceivedQPS", Collections.emptyMap());
        metrics.put("TableSourceReceivedBytesPerSeconds", Collections.emptyMap());
        metrics.put("TableSinkWriteBytes", Collections.emptyMap());
        metrics.put("TableSinkWriteCount", Collections.emptyMap());
        metrics.put("TableSinkWriteQPS", Collections.emptyMap());
        metrics.put("TableSinkWriteBytesPerSeconds", Collections.emptyMap());
        return metrics;
    }
}
