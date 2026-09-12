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

import org.apache.seatunnel.common.utils.JsonUtils;

import io.github.leonyoah.stx.proxy.model.DagParseResult;
import io.github.leonyoah.stx.proxy.model.JobConfigContext;
import io.github.leonyoah.stx.proxy.model.NodeKind;
import io.github.leonyoah.stx.proxy.model.ProxyEdge;
import io.github.leonyoah.stx.proxy.model.ProxyNode;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.net.URLDecoder;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.Deque;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

public class PreviewConfigService {

    private static final String PREVIEW_SOURCE_DATASET_PREFIX = "__st_preview_source_";
    private static final String PREVIEW_TRANSFORM_DATASET_PREFIX = "__st_preview_transform_";
    private static final String PREVIEW_HTTP_PLUGIN = "Http";
    private static final String PREVIEW_METADATA_PLUGIN = "Metadata";
    private static final String PREVIEW_METADATA_OUTPUT = "__st_preview_rows";

    private final JobConfigSupportService jobConfigSupportService;
    private final ConfigResourceService configResourceService;

    public PreviewConfigService() {
        this(new JobConfigSupportService());
    }

    PreviewConfigService(JobConfigSupportService jobConfigSupportService) {
        this.jobConfigSupportService = jobConfigSupportService;
        this.configResourceService = new ConfigResourceService(jobConfigSupportService);
    }

    public Map<String, Object> deriveSourcePreview(Map<String, Object> request) {
        JobConfigContext context = jobConfigSupportService.parseJobContext(request);
        boolean explicitSourceSelection =
                ProxyRequestUtils.getOptionalString(request, "sourceNodeId") != null
                        || request.get("sourceIndex") != null;
        if (!explicitSourceSelection && context.getSources().size() > 1) {
            return deriveWholeGraphPreview(request, context);
        }
        int sourceIndex =
                resolveNodeIndex(
                        request,
                        "sourceNodeId",
                        "sourceIndex",
                        NodeKind.SOURCE,
                        context.getSources().size());

        String sourceNodeId = jobConfigSupportService.nodeId(NodeKind.SOURCE, sourceIndex);
        String sourceOutput = PREVIEW_SOURCE_DATASET_PREFIX + sourceIndex;
        List<Map<String, Object>> sources =
                Collections.singletonList(
                        normalizeSourceConfig(context.getSources().get(sourceIndex), sourceOutput));
        String metadataOutput =
                ProxyRequestUtils.getOptionalString(request, "metadataOutputDataset");
        if (metadataOutput == null) {
            metadataOutput = PREVIEW_METADATA_OUTPUT;
        }
        List<Map<String, Object>> transforms =
                Collections.singletonList(
                        buildMetadataTransform(
                                sourceOutput, metadataOutput, resolveMetadataFields(request)));
        List<Map<String, Object>> sinks =
                Collections.singletonList(buildHttpSink(request, metadataOutput));

        return buildPreviewResponse(
                "source_preview",
                context,
                sourceNodeId,
                sourceIndex,
                sources,
                transforms,
                sinks,
                request);
    }

    public Map<String, Object> deriveTransformPreview(Map<String, Object> request) {
        JobConfigContext context = jobConfigSupportService.parseJobContext(request);
        if (context.getTransforms().isEmpty()) {
            throw new ProxyException(400, "Transform preview requires at least one transform");
        }
        int transformIndex =
                resolveNodeIndex(
                        request,
                        "transformNodeId",
                        "transformIndex",
                        NodeKind.TRANSFORM,
                        context.getTransforms().size());
        String targetNodeId = jobConfigSupportService.nodeId(NodeKind.TRANSFORM, transformIndex);
        Set<String> retainedNodeIds = collectUpstreamNodes(targetNodeId, context.getGraph());

        Map<String, String> normalizedOutputs = new LinkedHashMap<>();
        List<Map<String, Object>> sources = new ArrayList<>();
        for (int i = 0; i < context.getSources().size(); i++) {
            String nodeId = jobConfigSupportService.nodeId(NodeKind.SOURCE, i);
            if (!retainedNodeIds.contains(nodeId)) {
                continue;
            }
            String output = PREVIEW_SOURCE_DATASET_PREFIX + i;
            normalizedOutputs.put(nodeId, output);
            sources.add(normalizeSourceConfig(context.getSources().get(i), output));
        }

        List<Map<String, Object>> transforms = new ArrayList<>();
        for (int i = 0; i < context.getTransforms().size(); i++) {
            String nodeId = jobConfigSupportService.nodeId(NodeKind.TRANSFORM, i);
            if (!retainedNodeIds.contains(nodeId)) {
                continue;
            }
            String output = PREVIEW_TRANSFORM_DATASET_PREFIX + i;
            normalizedOutputs.put(nodeId, output);
        }
        for (int i = 0; i < context.getTransforms().size(); i++) {
            String nodeId = jobConfigSupportService.nodeId(NodeKind.TRANSFORM, i);
            if (!retainedNodeIds.contains(nodeId)) {
                continue;
            }
            ProxyNode node = getNode(context, nodeId);
            transforms.add(
                    normalizeTransformConfig(
                            context.getTransforms().get(i),
                            node,
                            context.getGraph().getEdges(),
                            normalizedOutputs));
        }

        String metadataOutput =
                ProxyRequestUtils.getOptionalString(request, "metadataOutputDataset");
        if (metadataOutput == null) {
            metadataOutput = PREVIEW_METADATA_OUTPUT;
        }
        transforms.add(
                buildMetadataTransform(
                        normalizedOutputs.get(targetNodeId),
                        metadataOutput,
                        resolveMetadataFields(request)));

        List<Map<String, Object>> sinks =
                Collections.singletonList(buildHttpSink(request, metadataOutput));

        return buildPreviewResponse(
                "transform_preview",
                context,
                targetNodeId,
                transformIndex,
                sources,
                transforms,
                sinks,
                request);
    }

    private Map<String, Object> buildPreviewResponse(
            String mode,
            JobConfigContext context,
            String selectedNodeId,
            int selectedIndex,
            List<Map<String, Object>> sources,
            List<Map<String, Object>> transforms,
            List<Map<String, Object>> sinks,
            Map<String, Object> request) {
        Map<String, Object> derivedRoot = deriveBaseConfig(context.getJobConfig());
        derivedRoot.put("source", sources);
        derivedRoot.put("transform", transforms);
        derivedRoot.put("sink", sinks);
        applyEnvOverrides(derivedRoot, request);

        String outputFormat = resolveOutputFormat(request);
        String content = renderPreviewConfig(derivedRoot, outputFormat);
        String inspectContent = JsonUtils.toJsonString(derivedRoot);
        Map<String, Object> inspectRequest = new LinkedHashMap<>();
        inspectRequest.put("content", inspectContent);
        inspectRequest.put("contentFormat", "json");
        DagParseResult derivedDag = configResourceService.inspectDag(inspectRequest);

        Map<String, Object> response = new LinkedHashMap<>();
        response.put("ok", true);
        response.put("mode", mode);
        response.put("selectedNodeId", selectedNodeId);
        response.put("selectedIndex", selectedIndex);
        response.put("warnings", context.getWarnings());
        response.put("content", content);
        response.put("contentFormat", outputFormat);
        response.put("config", derivedRoot);
        response.put("graph", derivedDag.getGraph());
        response.put("simpleGraph", derivedDag.isSimpleGraph());
        return response;
    }

    private Map<String, Object> deriveWholeGraphPreview(
            Map<String, Object> request, JobConfigContext context) {
        List<Map<String, Object>> sources = new ArrayList<>();
        for (Config source : context.getSources()) {
            sources.add(configToMutableMap(source));
        }
        List<Map<String, Object>> transforms = new ArrayList<>();
        for (Config transform : context.getTransforms()) {
            transforms.add(configToMutableMap(transform));
        }

        LinkedHashSet<String> terminalDatasets = new LinkedHashSet<>();
        for (ProxyNode node : context.getGraph().getNodes()) {
            if (node.getKind() != NodeKind.SINK) {
                continue;
            }
            terminalDatasets.addAll(node.getInputDatasets());
        }
        if (terminalDatasets.isEmpty()) {
            for (ProxyNode node : context.getGraph().getNodes()) {
                if (node.getKind() == NodeKind.TRANSFORM && node.getOutputDataset() != null) {
                    terminalDatasets.add(node.getOutputDataset());
                }
            }
        }
        if (terminalDatasets.isEmpty()) {
            for (ProxyNode node : context.getGraph().getNodes()) {
                if (node.getKind() == NodeKind.SOURCE && node.getOutputDataset() != null) {
                    terminalDatasets.add(node.getOutputDataset());
                }
            }
        }
        if (terminalDatasets.isEmpty()) {
            throw new ProxyException(400, "Preview graph has no terminal dataset to collect");
        }

        List<Map<String, Object>> previewTransforms = new ArrayList<>(transforms);
        List<Map<String, Object>> sinks = new ArrayList<>();
        int metadataIndex = 0;
        for (String dataset : terminalDatasets) {
            String metadataOutput = PREVIEW_METADATA_OUTPUT + "_" + metadataIndex;
            previewTransforms.add(
                    buildMetadataTransform(
                            dataset, metadataOutput, resolveMetadataFields(request)));
            sinks.add(buildHttpSink(request, metadataOutput));
            metadataIndex++;
        }

        return buildPreviewResponse(
                "graph_preview", context, "all", -1, sources, previewTransforms, sinks, request);
    }

    private String renderPreviewConfig(Map<String, Object> derivedRoot, String outputFormat) {
        if ("json".equalsIgnoreCase(outputFormat)) {
            return JsonUtils.toJsonString(derivedRoot);
        }
        if ("hocon".equalsIgnoreCase(outputFormat)) {
            return renderPreviewHocon(derivedRoot);
        }
        throw new ProxyException(
                400, "Unsupported outputFormat: " + outputFormat + ", expected hocon or json");
    }

    private String renderPreviewHocon(Map<String, Object> derivedRoot) {
        StringBuilder builder = new StringBuilder();
        appendRootEntries(builder, derivedRoot);
        return builder.toString();
    }

    @SuppressWarnings("unchecked")
    private void appendRootEntries(StringBuilder builder, Map<String, Object> root) {
        boolean firstSection = true;
        for (Map.Entry<String, Object> entry : root.entrySet()) {
            if (!firstSection) {
                builder.append('\n');
            }
            String key = entry.getKey();
            Object value = entry.getValue();
            if (isPluginSection(key) && value instanceof List) {
                appendPluginSection(builder, key, (List<Object>) value, 0);
            } else if (value instanceof Map) {
                appendObjectBlock(builder, key, (Map<String, Object>) value, 0);
            } else {
                appendKeyValue(builder, key, value, 0);
            }
            firstSection = false;
        }
    }

    private boolean isPluginSection(String key) {
        return "source".equals(key) || "transform".equals(key) || "sink".equals(key);
    }

    @SuppressWarnings("unchecked")
    private void appendPluginSection(
            StringBuilder builder, String sectionName, List<Object> plugins, int indent) {
        appendIndent(builder, indent);
        builder.append(renderKey(sectionName)).append(" {\n");
        for (Object pluginObject : plugins) {
            if (!(pluginObject instanceof Map)) {
                throw new ProxyException(
                        500, "Preview plugin section contains non-object value: " + pluginObject);
            }
            Map<String, Object> plugin = (Map<String, Object>) pluginObject;
            Object pluginName = plugin.get("plugin_name");
            if (!(pluginName instanceof String)) {
                throw new ProxyException(
                        500, "Preview plugin section is missing plugin_name: " + plugin);
            }
            appendIndent(builder, indent + 2);
            builder.append(renderKey(String.valueOf(pluginName))).append(" {\n");
            for (Map.Entry<String, Object> entry : plugin.entrySet()) {
                if ("plugin_name".equals(entry.getKey())) {
                    continue;
                }
                appendEntry(builder, entry.getKey(), entry.getValue(), indent + 4);
            }
            appendIndent(builder, indent + 2);
            builder.append("}\n");
        }
        appendIndent(builder, indent);
        builder.append("}");
    }

    private void appendObjectBlock(
            StringBuilder builder, String key, Map<String, Object> mapValue, int indent) {
        appendIndent(builder, indent);
        builder.append(renderKey(key)).append(" {\n");
        for (Map.Entry<String, Object> entry : mapValue.entrySet()) {
            appendEntry(builder, entry.getKey(), entry.getValue(), indent + 2);
        }
        appendIndent(builder, indent);
        builder.append("}");
    }

    @SuppressWarnings("unchecked")
    private void appendEntry(StringBuilder builder, String key, Object value, int indent) {
        if (value instanceof Map) {
            appendObjectBlock(builder, key, (Map<String, Object>) value, indent);
            builder.append('\n');
            return;
        }
        appendKeyValue(builder, key, value, indent);
    }

    private void appendKeyValue(StringBuilder builder, String key, Object value, int indent) {
        appendIndent(builder, indent);
        builder.append(renderKey(key))
                .append(" = ")
                .append(renderValue(value, indent))
                .append('\n');
    }

    @SuppressWarnings("unchecked")
    private String renderValue(Object value, int indent) {
        if (value == null) {
            return "null";
        }
        if (value instanceof String) {
            return '"' + escapeString((String) value) + '"';
        }
        if (value instanceof Number || value instanceof Boolean) {
            return String.valueOf(value);
        }
        if (value instanceof Map) {
            return renderInlineObject((Map<String, Object>) value, indent);
        }
        if (value instanceof List) {
            return renderList((List<Object>) value, indent);
        }
        return '"' + escapeString(String.valueOf(value)) + '"';
    }

    private String renderList(List<Object> list, int indent) {
        if (list.isEmpty()) {
            return "[]";
        }
        boolean containsComplexValue = false;
        for (Object item : list) {
            if (item instanceof Map || item instanceof List) {
                containsComplexValue = true;
                break;
            }
        }
        if (!containsComplexValue) {
            StringBuilder builder = new StringBuilder("[");
            for (int i = 0; i < list.size(); i++) {
                if (i > 0) {
                    builder.append(", ");
                }
                builder.append(renderValue(list.get(i), indent));
            }
            builder.append("]");
            return builder.toString();
        }

        StringBuilder builder = new StringBuilder("[\n");
        for (int i = 0; i < list.size(); i++) {
            appendIndent(builder, indent + 2);
            builder.append(renderValue(list.get(i), indent + 2));
            if (i < list.size() - 1) {
                builder.append(',');
            }
            builder.append('\n');
        }
        appendIndent(builder, indent);
        builder.append(']');
        return builder.toString();
    }

    private String renderInlineObject(Map<String, Object> mapValue, int indent) {
        if (mapValue.isEmpty()) {
            return "{}";
        }
        StringBuilder builder = new StringBuilder("{\n");
        for (Map.Entry<String, Object> entry : mapValue.entrySet()) {
            appendIndent(builder, indent + 2);
            builder.append(renderKey(entry.getKey()))
                    .append(" = ")
                    .append(renderValue(entry.getValue(), indent + 2))
                    .append('\n');
        }
        appendIndent(builder, indent);
        builder.append('}');
        return builder.toString();
    }

    private void appendIndent(StringBuilder builder, int indent) {
        for (int i = 0; i < indent; i++) {
            builder.append(' ');
        }
    }

    private String renderKey(String key) {
        if (key.matches("[A-Za-z0-9_.-]+")) {
            return key;
        }
        return '"' + escapeString(key) + '"';
    }

    private String escapeString(String value) {
        return value.replace("\\", "\\\\").replace("\"", "\\\"");
    }

    private String resolveOutputFormat(Map<String, Object> request) {
        String outputFormat = ProxyRequestUtils.getOptionalString(request, "outputFormat");
        if (outputFormat == null) {
            return "hocon";
        }
        if ("json".equalsIgnoreCase(outputFormat) || "hocon".equalsIgnoreCase(outputFormat)) {
            return outputFormat;
        }
        throw new ProxyException(
                400, "Unsupported outputFormat: " + outputFormat + ", expected hocon or json");
    }

    private ProxyNode getNode(JobConfigContext context, String nodeId) {
        for (ProxyNode node : context.getGraph().getNodes()) {
            if (nodeId.equals(node.getNodeId())) {
                return node;
            }
        }
        throw new ProxyException(500, "Node not found in parsed graph: " + nodeId);
    }

    private Set<String> collectUpstreamNodes(
            String targetNodeId, io.github.leonyoah.stx.proxy.model.DatasetDag graph) {
        Set<String> retainedNodeIds = new LinkedHashSet<>();
        Deque<String> pending = new ArrayDeque<>();
        pending.push(targetNodeId);
        while (!pending.isEmpty()) {
            String currentNodeId = pending.pop();
            if (!retainedNodeIds.add(currentNodeId)) {
                continue;
            }
            for (ProxyEdge edge : graph.getEdges()) {
                if (currentNodeId.equals(edge.getToNodeId()) && edge.getFromNodeId() != null) {
                    pending.push(edge.getFromNodeId());
                }
            }
        }
        return retainedNodeIds;
    }

    private Map<String, Object> normalizeSourceConfig(Config config, String outputDataset) {
        Map<String, Object> sourceConfig = configToMutableMap(config);
        sourceConfig.put("plugin_output", outputDataset);
        return sourceConfig;
    }

    private Map<String, Object> normalizeTransformConfig(
            Config config,
            ProxyNode node,
            List<ProxyEdge> edges,
            Map<String, String> normalizedOutputs) {
        Map<String, Object> transformConfig = configToMutableMap(config);
        transformConfig.put(
                "plugin_input", resolveNormalizedInputs(node, edges, normalizedOutputs));
        transformConfig.put("plugin_output", normalizedOutputs.get(node.getNodeId()));
        return transformConfig;
    }

    private List<String> resolveNormalizedInputs(
            ProxyNode node, List<ProxyEdge> edges, Map<String, String> normalizedOutputs) {
        List<ProxyEdge> incomingEdges = new ArrayList<>();
        for (ProxyEdge edge : edges) {
            if (node.getNodeId().equals(edge.getToNodeId())) {
                incomingEdges.add(edge);
            }
        }
        if (incomingEdges.isEmpty()) {
            throw new ProxyException(500, "Selected transform has no upstream edge: " + node);
        }

        List<String> normalizedInputs = new ArrayList<>();
        Set<Integer> usedEdgeIndexes = new HashSet<>();
        for (String inputDataset : node.getInputDatasets()) {
            int matchedEdgeIndex = matchIncomingEdge(inputDataset, incomingEdges, usedEdgeIndexes);
            ProxyEdge edge = incomingEdges.get(matchedEdgeIndex);
            String normalizedOutput = normalizedOutputs.get(edge.getFromNodeId());
            if (normalizedOutput == null) {
                throw new ProxyException(
                        500, "Failed to resolve upstream dataset for node: " + node.getNodeId());
            }
            normalizedInputs.add(normalizedOutput);
            usedEdgeIndexes.add(matchedEdgeIndex);
        }

        if (normalizedInputs.isEmpty()) {
            for (ProxyEdge edge : incomingEdges) {
                String normalizedOutput = normalizedOutputs.get(edge.getFromNodeId());
                if (normalizedOutput != null) {
                    normalizedInputs.add(normalizedOutput);
                }
            }
        }
        return normalizedInputs;
    }

    private int matchIncomingEdge(
            String inputDataset, List<ProxyEdge> incomingEdges, Set<Integer> usedEdgeIndexes) {
        for (int i = 0; i < incomingEdges.size(); i++) {
            if (usedEdgeIndexes.contains(i)) {
                continue;
            }
            if (inputDataset.equals(incomingEdges.get(i).getFromDataset())) {
                return i;
            }
        }
        for (int i = 0; i < incomingEdges.size(); i++) {
            if (!usedEdgeIndexes.contains(i)) {
                return i;
            }
        }
        throw new ProxyException(500, "Unable to match incoming edge for dataset: " + inputDataset);
    }

    private Map<String, Object> buildMetadataTransform(
            String inputDataset, String outputDataset, Map<String, Object> metadataFields) {
        Map<String, Object> metadataTransform = new LinkedHashMap<>();
        metadataTransform.put("plugin_name", PREVIEW_METADATA_PLUGIN);
        metadataTransform.put("plugin_input", Collections.singletonList(inputDataset));
        metadataTransform.put("plugin_output", outputDataset);
        metadataTransform.put("metadata_fields", metadataFields);
        return metadataTransform;
    }

    private Map<String, Object> buildHttpSink(Map<String, Object> request, String inputDataset) {
        Map<String, Object> sinkRequest = ProxyRequestUtils.getMap(request, "httpSink");
        String url = ProxyRequestUtils.getOptionalString(sinkRequest, "url");
        if (url == null) {
            throw new ProxyException(400, "Preview derivation requires httpSink.url");
        }
        Integer previewRowLimit = resolvePreviewRowLimit(request);
        Map<String, Object> sinkConfig = deepCopyMap(sinkRequest);
        sinkConfig.put("url", injectPreviewQuery(url, request, previewRowLimit));
        sinkConfig.put("plugin_name", PREVIEW_HTTP_PLUGIN);
        sinkConfig.put("plugin_input", Collections.singletonList(inputDataset));
        return sinkConfig;
    }

    private String injectPreviewQuery(
            String url, Map<String, Object> request, Integer previewRowLimit) {
        String baseUrl = url;
        String rawQuery = "";
        int queryIndex = url.indexOf('?');
        if (queryIndex >= 0) {
            baseUrl = url.substring(0, queryIndex);
            rawQuery = url.substring(queryIndex + 1);
        }
        Map<String, String> query = new LinkedHashMap<>();
        if (!rawQuery.isEmpty()) {
            for (String part : rawQuery.split("&")) {
                if (part == null || part.trim().isEmpty()) {
                    continue;
                }
                String[] pieces = part.split("=", 2);
                String key = decodeQueryValue(pieces[0]);
                String value = pieces.length > 1 ? decodeQueryValue(pieces[1]) : "";
                query.put(key, value);
            }
        }
        String platformJobId = ProxyRequestUtils.getOptionalString(request, "platformJobId");
        String engineJobId = ProxyRequestUtils.getOptionalString(request, "engineJobId");
        if (platformJobId != null && !platformJobId.trim().isEmpty()) {
            query.put("platform_job_id", platformJobId.trim());
        }
        if (engineJobId != null && !engineJobId.trim().isEmpty()) {
            query.put("engine_job_id", engineJobId.trim());
        }
        if (previewRowLimit != null) {
            query.put("row_limit", String.valueOf(previewRowLimit));
        }
        StringBuilder builder = new StringBuilder(baseUrl);
        if (!query.isEmpty()) {
            builder.append('?');
            boolean first = true;
            for (Map.Entry<String, String> entry : query.entrySet()) {
                if (!first) {
                    builder.append('&');
                }
                builder.append(encodeQueryValue(entry.getKey()))
                        .append('=')
                        .append(encodeQueryValue(entry.getValue()));
                first = false;
            }
        }
        return builder.toString();
    }

    private String decodeQueryValue(String value) {
        try {
            return URLDecoder.decode(value, StandardCharsets.UTF_8.name());
        } catch (Exception e) {
            throw new ProxyException(400, "failed to decode preview query value", e);
        }
    }

    private String encodeQueryValue(String value) {
        try {
            return URLEncoder.encode(value, StandardCharsets.UTF_8.name());
        } catch (Exception e) {
            throw new ProxyException(500, "failed to encode preview query value", e);
        }
    }

    private Integer resolvePreviewRowLimit(Map<String, Object> request) {
        Object raw = request.get("previewRowLimit");
        if (raw == null) {
            return 100;
        }
        int value;
        if (raw instanceof Number) {
            value = ((Number) raw).intValue();
        } else {
            try {
                value = Integer.parseInt(String.valueOf(raw).trim());
            } catch (NumberFormatException e) {
                throw new ProxyException(400, "previewRowLimit must be an integer");
            }
        }
        if (value <= 0) {
            return 100;
        }
        return Math.min(value, 10000);
    }

    private Map<String, Object> resolveMetadataFields(Map<String, Object> request) {
        Map<String, Object> metadataFields = ProxyRequestUtils.getMap(request, "metadataFields");
        if (metadataFields.isEmpty()) {
            Map<String, Object> defaults = new LinkedHashMap<>();
            defaults.put("Database", "__st_debug_db");
            defaults.put("Table", "__st_debug_table");
            defaults.put("RowKind", "__st_debug_rowkind");
            return defaults;
        }
        return metadataFields;
    }

    private int resolveNodeIndex(
            Map<String, Object> request,
            String nodeField,
            String indexField,
            NodeKind kind,
            int size) {
        if (size <= 0) {
            throw new ProxyException(400, "No " + kind.name().toLowerCase() + " node is available");
        }
        String nodeId = ProxyRequestUtils.getOptionalString(request, nodeField);
        String rawIndex = ProxyRequestUtils.getOptionalString(request, indexField);
        Integer parsedIndex = rawIndex == null ? null : Integer.valueOf(rawIndex);

        if (nodeId == null && parsedIndex == null) {
            if (size == 1) {
                return 0;
            }
            throw new ProxyException(
                    400,
                    "Multiple "
                            + kind.name().toLowerCase()
                            + " nodes exist, specify either "
                            + nodeField
                            + " or "
                            + indexField);
        }

        if (nodeId != null) {
            String expectedPrefix = kind.name().toLowerCase() + "-";
            if (!nodeId.startsWith(expectedPrefix)) {
                throw new ProxyException(400, "Invalid " + nodeField + ": " + nodeId);
            }
            int nodeIndex = Integer.parseInt(nodeId.substring(expectedPrefix.length()));
            if (parsedIndex != null && parsedIndex.intValue() != nodeIndex) {
                throw new ProxyException(
                        400, "Conflicting " + nodeField + " and " + indexField + " values");
            }
            parsedIndex = nodeIndex;
        }

        if (parsedIndex == null || parsedIndex < 0 || parsedIndex >= size) {
            throw new ProxyException(400, "Selected " + indexField + " is out of range");
        }
        return parsedIndex;
    }

    private Map<String, Object> deriveBaseConfig(Config jobConfig) {
        Map<String, Object> root = deepCopyMap(jobConfig.root().unwrapped());
        root.remove("source");
        root.remove("transform");
        root.remove("sink");
        return root;
    }

    private void applyEnvOverrides(Map<String, Object> root, Map<String, Object> request) {
        Map<String, Object> envOverrides = ProxyRequestUtils.getMap(request, "envOverrides");
        if (envOverrides.isEmpty()) {
            return;
        }
        Map<String, Object> env =
                root.containsKey("env") ? toMutableMap(root.get("env")) : new LinkedHashMap<>();
        mergeMaps(env, envOverrides);
        root.put("env", env);
    }

    @SuppressWarnings("unchecked")
    private Map<String, Object> toMutableMap(Object value) {
        if (value == null) {
            return new LinkedHashMap<>();
        }
        if (!(value instanceof Map)) {
            throw new ProxyException(400, "Expected map value but found: " + value.getClass());
        }
        return deepCopyMap((Map<String, Object>) value);
    }

    @SuppressWarnings("unchecked")
    private Map<String, Object> configToMutableMap(Config config) {
        return deepCopyMap(config.root().unwrapped());
    }

    @SuppressWarnings("unchecked")
    private Map<String, Object> deepCopyMap(Map<String, Object> source) {
        Map<String, Object> copy = new LinkedHashMap<>();
        for (Map.Entry<String, Object> entry : source.entrySet()) {
            copy.put(entry.getKey(), deepCopy(entry.getValue()));
        }
        return copy;
    }

    @SuppressWarnings("unchecked")
    private Object deepCopy(Object value) {
        if (value instanceof Map) {
            return deepCopyMap((Map<String, Object>) value);
        }
        if (value instanceof List) {
            List<Object> copy = new ArrayList<>();
            for (Object item : (List<Object>) value) {
                copy.add(deepCopy(item));
            }
            return copy;
        }
        if (value != null && value.getClass().isArray()) {
            return Arrays.asList((Object[]) value);
        }
        return value;
    }

    @SuppressWarnings("unchecked")
    private void mergeMaps(Map<String, Object> target, Map<String, Object> overrides) {
        for (Map.Entry<String, Object> entry : overrides.entrySet()) {
            Object existing = target.get(entry.getKey());
            Object override = entry.getValue();
            if (existing instanceof Map && override instanceof Map) {
                mergeMaps((Map<String, Object>) existing, (Map<String, Object>) override);
            } else {
                target.put(entry.getKey(), deepCopy(override));
            }
        }
    }
}
