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

package io.github.leonyoah.stx.proxy.service.storage;

import org.apache.seatunnel.api.serialization.DefaultSerializer;
import org.apache.seatunnel.api.table.factory.ChangeStreamTableSourceCheckpoint;
import org.apache.seatunnel.api.table.factory.ChangeStreamTableSourceFactory;
import org.apache.seatunnel.api.table.factory.ChangeStreamTableSourceState;
import org.apache.seatunnel.engine.checkpoint.storage.PipelineState;
import org.apache.seatunnel.engine.serializer.protobuf.ProtoStuffSerializer;

import org.apache.hadoop.conf.Configuration;
import org.apache.hadoop.fs.FSDataInputStream;
import org.apache.hadoop.fs.FileStatus;
import org.apache.hadoop.fs.FileSystem;
import org.apache.hadoop.fs.Path;

import io.github.leonyoah.stx.proxy.service.plugin.PluginClassLoaderUtils;
import io.github.leonyoah.stx.proxy.service.support.ProbeExecutionUtils;
import io.github.leonyoah.stx.proxy.service.support.ProxyException;
import io.github.leonyoah.stx.proxy.service.support.ProxyRequestUtils;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.Serializable;
import java.lang.reflect.Array;
import java.lang.reflect.Method;
import java.net.URLClassLoader;
import java.util.ArrayList;
import java.util.Base64;
import java.util.Collection;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;

public class CheckpointSourceStateInspectService {

    private static final String COMPLETED_CHECKPOINT_CLASS =
            "org.apache.seatunnel.engine.server.checkpoint.CompletedCheckpoint";
    private static final int DEFAULT_SPLIT_LIMIT_PER_SUBTASK = 20;

    private final org.apache.seatunnel.engine.serializer.api.Serializer serializer =
            new ProtoStuffSerializer();
    private final StreamingSourceDescriptorRegistry descriptorRegistry =
            new StreamingSourceDescriptorRegistry();
    private final CheckpointSourceActionMatcher actionMatcher = new CheckpointSourceActionMatcher();

    public Map<String, Object> inspect(Map<String, Object> request) {
        return ProbeExecutionUtils.runWithTimeout(
                request,
                "Checkpoint source state inspect",
                "Check checkpoint path, plugin jars, and jobConfig source targeting.",
                () -> doInspect(request));
    }

    private Map<String, Object> doInspect(Map<String, Object> request) throws IOException {
        List<String> pluginJars = ProxyRequestUtils.getStringList(request, "pluginJars");
        ClassLoader parent = Thread.currentThread().getContextClassLoader();
        URLClassLoader urlClassLoader = null;
        try {
            ClassLoader runtimeClassLoader = parent;
            if (!pluginJars.isEmpty()) {
                urlClassLoader = PluginClassLoaderUtils.createClassLoader(pluginJars, parent);
                runtimeClassLoader = urlClassLoader;
            }
            Thread currentThread = Thread.currentThread();
            ClassLoader originalClassLoader = currentThread.getContextClassLoader();
            currentThread.setContextClassLoader(runtimeClassLoader);
            try {
                byte[] rawBytes = loadRawBytes(request);
                PipelineState pipelineState = serializer.deserialize(rawBytes, PipelineState.class);
                Object checkpoint =
                        deserializeCompletedCheckpoint(
                                pipelineState.getStates(), runtimeClassLoader);
                Map<?, ?> actionStates = asMap(invoke(checkpoint, "getTaskStates"));

                List<String> warnings = new ArrayList<>();
                List<Map<String, Object>> sources = new ArrayList<>();
                List<Map<String, Object>> unsupportedSources = new ArrayList<>();
                List<CheckpointSourceActionMatcher.SourceTarget> targets =
                        actionMatcher.match(request);
                int splitLimitPerSubtask =
                        Math.max(
                                1,
                                (int)
                                        ProxyRequestUtils.getLong(
                                                request,
                                                "splitLimitPerSubtask",
                                                DEFAULT_SPLIT_LIMIT_PER_SUBTASK));
                boolean includeCoordinator =
                        ProxyRequestUtils.getBoolean(request, "includeCoordinator", true);
                boolean includeSubtaskSplits =
                        ProxyRequestUtils.getBoolean(request, "includeSubtaskSplits", true);

                for (CheckpointSourceActionMatcher.SourceTarget target : targets) {
                    Map<String, Object> actionEntry =
                            findActionEntry(actionStates, target.getActionName());
                    if (actionEntry == null) {
                        unsupportedSources.add(
                                buildUnsupported(
                                        target,
                                        "ACTION_STATE_MISSING",
                                        "Checkpoint action state not found"));
                        continue;
                    }
                    StreamingSourceDescriptorRegistry.StreamingSourceDescriptor descriptor =
                            descriptorRegistry.find(target.getPluginName());
                    if (descriptor == null) {
                        unsupportedSources.add(
                                buildUnsupported(
                                        target,
                                        "DESCRIPTOR_MISSING",
                                        "No official descriptor registered for source"));
                        continue;
                    }
                    try {
                        sources.add(
                                decodeSource(
                                        target,
                                        descriptor,
                                        actionEntry,
                                        runtimeClassLoader,
                                        splitLimitPerSubtask,
                                        includeCoordinator,
                                        includeSubtaskSplits,
                                        warnings));
                    } catch (Exception e) {
                        unsupportedSources.add(
                                buildUnsupported(
                                        target,
                                        "DECODE_FAILED",
                                        e.getMessage() == null
                                                ? e.getClass().getName()
                                                : e.getMessage()));
                    }
                }

                Map<String, Object> response = new LinkedHashMap<>();
                response.put("ok", true);
                response.put("pipelineState", buildPipelineState(pipelineState));
                response.put("completedCheckpoint", buildCompletedCheckpoint(checkpoint));
                response.put("sources", sources);
                response.put("unsupportedSources", unsupportedSources);
                response.put("warnings", warnings);
                return response;
            } finally {
                currentThread.setContextClassLoader(originalClassLoader);
            }
        } finally {
            PluginClassLoaderUtils.closeQuietly(urlClassLoader);
        }
    }

    private Map<String, Object> decodeSource(
            CheckpointSourceActionMatcher.SourceTarget target,
            StreamingSourceDescriptorRegistry.StreamingSourceDescriptor descriptor,
            Map<String, Object> actionEntry,
            ClassLoader classLoader,
            int splitLimitPerSubtask,
            boolean includeCoordinator,
            boolean includeSubtaskSplits,
            List<String> warnings)
            throws Exception {
        Object actionState = actionEntry.get("actionState");
        byte[] coordinatorBytes = firstStateChunk(invoke(actionState, "getCoordinatorState"));
        List<?> subtaskStates = asList(invoke(actionState, "getSubtaskStates"));
        List<List<byte[]>> splitBytes = new ArrayList<>();
        if (subtaskStates != null) {
            for (Object subtaskState : subtaskStates) {
                splitBytes.add(copyStateChunks(subtaskState));
            }
        }

        Object coordinatorStateObject = null;
        List<List<Object>> splitObjects = new ArrayList<>();
        if (descriptor.getDecodeStrategy()
                == StreamingSourceDescriptorRegistry.DecodeStrategy.CHANGE_STREAM_FACTORY) {
            ChangeStreamTableSourceState<?, ?> state =
                    decodeChangeStreamFactory(
                            descriptor, coordinatorBytes, splitBytes, classLoader);
            coordinatorStateObject = state == null ? null : state.getEnumeratorState();
            if (state != null && state.getSplits() != null) {
                for (List<?> subtaskSplits : state.getSplits()) {
                    List<Object> items = new ArrayList<>();
                    if (subtaskSplits != null) {
                        items.addAll(subtaskSplits);
                    }
                    splitObjects.add(items);
                }
            }
        } else {
            if (coordinatorBytes != null) {
                coordinatorStateObject = deserializeDefault(coordinatorBytes);
            }
            for (List<byte[]> subtaskChunks : splitBytes) {
                List<Object> items = new ArrayList<>();
                for (byte[] chunk : subtaskChunks) {
                    if (chunk != null) {
                        items.add(deserializeDefault(chunk));
                    }
                }
                splitObjects.add(items);
            }
        }

        Map<String, Object> source = new LinkedHashMap<>();
        source.put("configIndex", target.getConfigIndex());
        source.put("pluginName", target.getPluginName());
        source.put("actionName", target.getActionName());
        source.put("decodeStrategy", descriptor.getDecodeStrategy().name());
        source.put(
                "enumeratorStateClass",
                classNameOrDefault(
                        coordinatorStateObject, descriptor.getEnumeratorStateClassName()));
        source.put(
                "subtaskStateClass",
                resolveSubtaskClass(splitObjects, descriptor.getSplitClassName()));
        if (includeCoordinator) {
            source.put(
                    "coordinator",
                    projectCoordinator(
                            descriptor.getProjectorId(), coordinatorStateObject, warnings));
        }
        List<Map<String, Object>> subtasks = new ArrayList<>();
        List<?> rawSubtasks = subtaskStates == null ? Collections.emptyList() : subtaskStates;
        for (int i = 0; i < rawSubtasks.size(); i++) {
            Object subtaskState = rawSubtasks.get(i);
            List<Object> decodedSplits =
                    i < splitObjects.size() ? splitObjects.get(i) : Collections.emptyList();
            Map<String, Object> subtask = new LinkedHashMap<>();
            subtask.put("subtaskIndex", invoke(subtaskState, "getIndex"));
            subtask.put("splitCount", decodedSplits.size());
            subtask.put("chunks", stateChunkCount(subtaskState));
            subtask.put("bytes", stateChunkBytes(subtaskState));
            if (includeSubtaskSplits) {
                List<Map<String, Object>> splitSummaries = new ArrayList<>();
                int count = 0;
                for (Object split : decodedSplits) {
                    if (count >= splitLimitPerSubtask) {
                        break;
                    }
                    splitSummaries.add(projectSplit(descriptor.getProjectorId(), split, warnings));
                    count++;
                }
                subtask.put("splits", splitSummaries);
                subtask.put("truncated", decodedSplits.size() > splitLimitPerSubtask);
            }
            subtasks.add(subtask);
        }
        source.put("subtasks", subtasks);
        return source;
    }

    private ChangeStreamTableSourceState<?, ?> decodeChangeStreamFactory(
            StreamingSourceDescriptorRegistry.StreamingSourceDescriptor descriptor,
            byte[] coordinatorBytes,
            List<List<byte[]>> splitBytes,
            ClassLoader classLoader)
            throws Exception {
        if (descriptor.getFactoryClassName() == null
                || descriptor.getFactoryClassName().trim().isEmpty()) {
            throw new ProxyException(500, "Factory class is required for change stream decoder");
        }
        Class<?> factoryClass = Class.forName(descriptor.getFactoryClassName(), true, classLoader);
        Object factory = factoryClass.getDeclaredConstructor().newInstance();
        if (!(factory instanceof ChangeStreamTableSourceFactory)) {
            throw new ProxyException(
                    500,
                    "Factory does not implement ChangeStreamTableSourceFactory: "
                            + descriptor.getFactoryClassName());
        }
        ChangeStreamTableSourceCheckpoint checkpoint =
                new ChangeStreamTableSourceCheckpoint(coordinatorBytes, splitBytes);
        return ((ChangeStreamTableSourceFactory) factory).deserializeTableSourceState(checkpoint);
    }

    private Object deserializeDefault(byte[] bytes) throws IOException {
        org.apache.seatunnel.api.serialization.Serializer<Serializable> serializer =
                new DefaultSerializer<>();
        return serializer.deserialize(bytes);
    }

    private Map<String, Object> projectCoordinator(
            String projectorId, Object coordinatorState, List<String> warnings) {
        if (coordinatorState == null) {
            return null;
        }
        if ("cdc-projector".equals(projectorId)) {
            return projectCdcCoordinator(coordinatorState);
        }
        if ("kafka-projector".equals(projectorId)) {
            return projectKafkaCoordinator(coordinatorState);
        }
        if ("pulsar-projector".equals(projectorId)) {
            return projectPulsarCoordinator(coordinatorState);
        }
        if ("rocketmq-projector".equals(projectorId)) {
            return projectRocketMqCoordinator(coordinatorState);
        }
        if ("rabbitmq-projector".equals(projectorId)) {
            return projectRabbitMqCoordinator(coordinatorState);
        }
        if ("sls-projector".equals(projectorId)) {
            return projectSlsCoordinator(coordinatorState);
        }
        if ("tablestore-projector".equals(projectorId)) {
            return projectTableStoreCoordinator(coordinatorState);
        }
        if ("paimon-projector".equals(projectorId)) {
            return projectPaimonCoordinator(coordinatorState);
        }
        if ("iceberg-projector".equals(projectorId)) {
            return projectIcebergCoordinator(coordinatorState);
        }
        if ("fake-projector".equals(projectorId)) {
            return projectFakeCoordinator(coordinatorState);
        }
        if ("single-split-projector".equals(projectorId)) {
            return projectSingleSplitCoordinator(coordinatorState);
        }
        if ("tidb-projector".equals(projectorId)) {
            return projectTidbCoordinator(coordinatorState);
        }
        warnings.add("No specialized projector for " + projectorId + ", using generic summary");
        return projectGenericState(coordinatorState);
    }

    private Map<String, Object> projectSplit(
            String projectorId, Object split, List<String> warnings) {
        if (split == null) {
            return null;
        }
        if ("cdc-projector".equals(projectorId)) {
            return projectCdcSplit(split);
        }
        if ("kafka-projector".equals(projectorId)) {
            return projectKafkaSplit(split);
        }
        if ("pulsar-projector".equals(projectorId)) {
            return projectPulsarSplit(split);
        }
        if ("rocketmq-projector".equals(projectorId)) {
            return projectRocketMqSplit(split);
        }
        if ("rabbitmq-projector".equals(projectorId)) {
            return projectRabbitMqSplit(split);
        }
        if ("sls-projector".equals(projectorId)) {
            return projectSlsSplit(split);
        }
        if ("tablestore-projector".equals(projectorId)) {
            return projectTableStoreSplit(split);
        }
        if ("paimon-projector".equals(projectorId)) {
            return projectPaimonSplit(split);
        }
        if ("iceberg-projector".equals(projectorId)) {
            return projectIcebergSplit(split);
        }
        if ("fake-projector".equals(projectorId)) {
            return projectFakeSplit(split);
        }
        if ("single-split-projector".equals(projectorId)) {
            return projectSingleSplitSplit(split);
        }
        if ("tidb-projector".equals(projectorId)) {
            return projectTidbSplit(split);
        }
        warnings.add(
                "No specialized split projector for " + projectorId + ", using generic summary");
        return projectGenericState(split);
    }

    private Map<String, Object> projectCdcCoordinator(Object coordinatorState) {
        Map<String, Object> result = baseProjection(coordinatorState);
        Object snapshotPhaseState = invoke(coordinatorState, "getSnapshotPhaseState");
        Object incrementalPhaseState = invoke(coordinatorState, "getIncrementalPhaseState");
        if (snapshotPhaseState != null) {
            Map<String, Object> snapshot = new LinkedHashMap<>();
            snapshot.put(
                    "remainingSplitCount",
                    sizeOfCollection(
                            asCollection(invoke(snapshotPhaseState, "getRemainingSplits"))));
            snapshot.put(
                    "assignedSplitCount",
                    sizeOfMap(asMap(invoke(snapshotPhaseState, "getAssignedSplits"))));
            snapshot.put(
                    "alreadyProcessedTables",
                    toStringList(
                            asCollection(invoke(snapshotPhaseState, "getAlreadyProcessedTables"))));
            snapshot.put("assignerCompleted", invoke(snapshotPhaseState, "isAssignerCompleted"));
            result.put("snapshotPhase", snapshot);
        }
        if (incrementalPhaseState != null) {
            Map<String, Object> incremental = new LinkedHashMap<>();
            incremental.put("className", incrementalPhaseState.getClass().getName());
            result.put("incrementalPhase", incremental);
        }
        return result;
    }

    private Map<String, Object> projectCdcSplit(Object split) {
        Map<String, Object> result = baseProjection(split);
        result.put("splitId", invoke(split, "splitId"));
        result.put("tableIds", toStringList(asCollection(invoke(split, "getTableIds"))));
        result.put("startupOffset", projectOffset(invoke(split, "getStartupOffset")));
        result.put("stopOffset", projectOffset(invoke(split, "getStopOffset")));
        result.put(
                "completedSnapshotSplitInfoCount",
                sizeOfCollection(asCollection(invoke(split, "getCompletedSnapshotSplitInfos"))));
        result.put(
                "checkpointTableCount",
                sizeOfCollection(asCollection(invoke(split, "getCheckpointTables"))));
        return result;
    }

    private Map<String, Object> projectKafkaCoordinator(Object coordinatorState) {
        Map<String, Object> result = baseProjection(coordinatorState);
        Collection<?> assignedSplit = asCollection(invoke(coordinatorState, "getAssignedSplit"));
        result.put("assignedSplitCount", sizeOfCollection(assignedSplit));
        List<Map<String, Object>> splits = new ArrayList<>();
        if (assignedSplit != null) {
            for (Object split : assignedSplit) {
                splits.add(projectKafkaSplit(split));
            }
        }
        result.put("assignedSplits", splits);
        return result;
    }

    private Map<String, Object> projectKafkaSplit(Object split) {
        Map<String, Object> result = baseProjection(split);
        result.put("splitId", invoke(split, "splitId"));
        Object topicPartition = invoke(split, "getTopicPartition");
        result.put("topic", invoke(topicPartition, "topic"));
        result.put("partition", invoke(topicPartition, "partition"));
        result.put("startOffset", invoke(split, "getStartOffset"));
        result.put("endOffset", invoke(split, "getEndOffset"));
        Object currentOffset = invoke(split, "getCurrentOffset");
        if (currentOffset != null) {
            result.put("currentOffset", currentOffset);
        }
        Object tablePath = invoke(split, "getTablePath");
        if (tablePath != null) {
            result.put("tablePath", String.valueOf(tablePath));
        }
        return result;
    }

    private Map<String, Object> projectPulsarCoordinator(Object coordinatorState) {
        Map<String, Object> result = baseProjection(coordinatorState);
        Collection<?> assignedPartitions =
                asCollection(invoke(coordinatorState, "getAssignedPartitions"));
        result.put("assignedPartitionCount", sizeOfCollection(assignedPartitions));
        result.put("assignedPartitions", toStringList(assignedPartitions));
        return result;
    }

    private Map<String, Object> projectPulsarSplit(Object split) {
        Map<String, Object> result = baseProjection(split);
        result.put("splitId", invoke(split, "splitId"));
        Object partition = invoke(split, "getPartition");
        if (partition != null) {
            result.put("partition", String.valueOf(partition));
            result.put("topic", invoke(partition, "getFullTopicName"));
        }
        Object latestConsumedId = invoke(split, "getLatestConsumedId");
        if (latestConsumedId != null) {
            result.put("latestConsumedId", String.valueOf(latestConsumedId));
        }
        Object stopCursor = invoke(split, "getStopCursor");
        if (stopCursor != null) {
            result.put("stopCursor", String.valueOf(stopCursor));
        }
        return result;
    }

    private Map<String, Object> projectRocketMqCoordinator(Object coordinatorState) {
        Map<String, Object> result = baseProjection(coordinatorState);
        Collection<?> assignedSplits = asCollection(invoke(coordinatorState, "getAssignSplits"));
        result.put("assignedSplitCount", sizeOfCollection(assignedSplits));
        List<Map<String, Object>> splits = new ArrayList<>();
        if (assignedSplits != null) {
            for (Object split : assignedSplits) {
                splits.add(projectRocketMqSplit(split));
            }
        }
        result.put("assignedSplits", splits);
        return result;
    }

    private Map<String, Object> projectRocketMqSplit(Object split) {
        Map<String, Object> result = baseProjection(split);
        result.put("splitId", invoke(split, "splitId"));
        result.put("startOffset", invoke(split, "getStartOffset"));
        result.put("endOffset", invoke(split, "getEndOffset"));
        Object queue = invoke(split, "getMessageQueue");
        if (queue != null) {
            result.put("topic", invoke(queue, "getTopic"));
            result.put("brokerName", invoke(queue, "getBrokerName"));
            result.put("queueId", invoke(queue, "getQueueId"));
        }
        return result;
    }

    private Map<String, Object> projectRabbitMqCoordinator(Object coordinatorState) {
        Map<String, Object> result = baseProjection(coordinatorState);
        result.put("enumeratorState", "stateless");
        return result;
    }

    private Map<String, Object> projectRabbitMqSplit(Object split) {
        Map<String, Object> result = baseProjection(split);
        Collection<?> deliveryTags = asCollection(invoke(split, "getDeliveryTags"));
        Collection<?> correlationIds = asCollection(invoke(split, "getCorrelationIds"));
        result.put("splitId", invoke(split, "splitId"));
        result.put("deliveryTagCount", sizeOfCollection(deliveryTags));
        result.put("correlationIdCount", sizeOfCollection(correlationIds));
        result.put("correlationIds", toStringList(correlationIds));
        return result;
    }

    private Map<String, Object> projectSlsCoordinator(Object coordinatorState) {
        Map<String, Object> result = baseProjection(coordinatorState);
        Collection<?> assignedSplit = asCollection(invoke(coordinatorState, "getAssignedSplit"));
        result.put("assignedSplitCount", sizeOfCollection(assignedSplit));
        return result;
    }

    private Map<String, Object> projectSlsSplit(Object split) {
        Map<String, Object> result = baseProjection(split);
        result.put("splitId", invoke(split, "splitId"));
        result.put("project", invoke(split, "getProject"));
        result.put("logStore", invoke(split, "getLogStore"));
        result.put("consumer", invoke(split, "getConsumer"));
        result.put("shardId", invoke(split, "getShardId"));
        result.put("startCursor", invoke(split, "getStartCursor"));
        result.put("fetchSize", invoke(split, "getFetchSize"));
        return result;
    }

    private Map<String, Object> projectTableStoreCoordinator(Object coordinatorState) {
        Map<String, Object> result = baseProjection(coordinatorState);
        result.put("shouldEnumerate", invoke(coordinatorState, "isShouldEnumerate"));
        Map<?, ?> pendingSplits = asMap(invoke(coordinatorState, "getPendingSplits"));
        result.put("pendingReaderCount", sizeOfMap(pendingSplits));
        int splitCount = 0;
        if (pendingSplits != null) {
            for (Object item : pendingSplits.values()) {
                splitCount += sizeOfCollection(asCollection(item));
            }
        }
        result.put("pendingSplitCount", splitCount);
        return result;
    }

    private Map<String, Object> projectTableStoreSplit(Object split) {
        Map<String, Object> result = baseProjection(split);
        result.put("splitId", invoke(split, "splitId"));
        result.put("tableName", invoke(split, "getTableName"));
        result.put("primaryKey", invoke(split, "getPrimaryKey"));
        return result;
    }

    private Map<String, Object> projectPaimonCoordinator(Object coordinatorState) {
        Map<String, Object> result = baseProjection(coordinatorState);
        Collection<?> assignedSplits = asCollection(invoke(coordinatorState, "getAssignedSplits"));
        result.put("assignedSplitCount", sizeOfCollection(assignedSplits));
        result.put("currentSnapshotId", invoke(coordinatorState, "getCurrentSnapshotId"));
        return result;
    }

    private Map<String, Object> projectPaimonSplit(Object split) {
        Map<String, Object> result = baseProjection(split);
        result.put("splitId", invoke(split, "splitId"));
        result.put("id", invoke(split, "getId"));
        result.put("tableId", invoke(split, "getTableId"));
        Object wrappedSplit = invoke(split, "getSplit");
        if (wrappedSplit != null) {
            result.put("wrappedSplit", String.valueOf(wrappedSplit));
        }
        return result;
    }

    private Map<String, Object> projectIcebergCoordinator(Object coordinatorState) {
        Map<String, Object> result = baseProjection(coordinatorState);
        Collection<?> pendingTables = asCollection(invoke(coordinatorState, "getPendingTables"));
        result.put("pendingTableCount", sizeOfCollection(pendingTables));
        result.put("pendingTables", toStringList(pendingTables));
        Map<?, ?> pendingSplits = asMap(invoke(coordinatorState, "getPendingSplits"));
        result.put("pendingReaderCount", sizeOfMap(pendingSplits));
        int splitCount = 0;
        if (pendingSplits != null) {
            for (Object item : pendingSplits.values()) {
                splitCount += sizeOfCollection(asCollection(item));
            }
        }
        result.put("pendingSplitCount", splitCount);
        Map<?, ?> tableOffsets = asMap(invoke(coordinatorState, "getTableOffsets"));
        result.put("tableOffsetCount", sizeOfMap(tableOffsets));
        if (tableOffsets != null && !tableOffsets.isEmpty()) {
            Map<String, Object> offsets = new LinkedHashMap<>();
            for (Map.Entry<?, ?> entry : tableOffsets.entrySet()) {
                offsets.put(String.valueOf(entry.getKey()), String.valueOf(entry.getValue()));
            }
            result.put("tableOffsets", offsets);
        }
        return result;
    }

    private Map<String, Object> projectIcebergSplit(Object split) {
        Map<String, Object> result = baseProjection(split);
        result.put("splitId", invoke(split, "splitId"));
        Object tablePath = invoke(split, "getTablePath");
        if (tablePath != null) {
            result.put("tablePath", String.valueOf(tablePath));
        }
        result.put("recordOffset", invoke(split, "getRecordOffset"));
        Object task = invoke(split, "getTask");
        if (task != null) {
            result.put("task", String.valueOf(task));
            Object file = invoke(task, "file");
            if (file != null) {
                Object path = invoke(file, "path");
                if (path != null) {
                    result.put("file", String.valueOf(path));
                }
            }
            result.put("start", invoke(task, "start"));
            result.put("length", invoke(task, "length"));
            Collection<?> deletes = asCollection(invoke(task, "deletes"));
            result.put("deleteFileCount", sizeOfCollection(deletes));
        }
        return result;
    }

    private Map<String, Object> projectFakeCoordinator(Object coordinatorState) {
        Map<String, Object> result = baseProjection(coordinatorState);
        Collection<?> assignedSplits = asCollection(invoke(coordinatorState, "getAssignedSplits"));
        result.put("assignedSplitCount", sizeOfCollection(assignedSplits));
        return result;
    }

    private Map<String, Object> projectFakeSplit(Object split) {
        Map<String, Object> result = baseProjection(split);
        result.put("splitId", invoke(split, "splitId"));
        result.put("tableId", invoke(split, "getTableId"));
        result.put("rowNum", invoke(split, "getRowNum"));
        return result;
    }

    private Map<String, Object> projectSingleSplitCoordinator(Object coordinatorState) {
        Map<String, Object> result = baseProjection(coordinatorState);
        result.put("recoverablePayload", false);
        return result;
    }

    private Map<String, Object> projectSingleSplitSplit(Object split) {
        Map<String, Object> result = baseProjection(split);
        byte[] state = asByteArray(invoke(split, "getState"));
        result.put("splitId", invoke(split, "splitId"));
        result.put("hasPayload", state != null && state.length > 0);
        result.put("payloadSizeBytes", state == null ? 0 : state.length);
        return result;
    }

    private Map<String, Object> projectTidbCoordinator(Object coordinatorState) {
        Map<String, Object> result = baseProjection(coordinatorState);
        result.put("shouldEnumerate", invoke(coordinatorState, "isShouldEnumerate"));
        Map<?, ?> pendingSplits = asMap(invoke(coordinatorState, "getPendingSplit"));
        result.put("pendingSplitCount", sizeOfMap(pendingSplits));
        return result;
    }

    private Map<String, Object> projectTidbSplit(Object split) {
        Map<String, Object> result = baseProjection(split);
        result.put("splitId", invoke(split, "splitId"));
        result.put("database", invoke(split, "getDatabase"));
        result.put("table", invoke(split, "getTable"));
        result.put("resolvedTs", invoke(split, "getResolvedTs"));
        result.put("snapshotCompleted", invoke(split, "isSnapshotCompleted"));
        return result;
    }

    private Map<String, Object> projectGenericState(Object state) {
        Map<String, Object> result = baseProjection(state);
        for (String methodName :
                new String[] {
                    "splitId",
                    "getStartOffset",
                    "getEndOffset",
                    "getCurrentOffset",
                    "getResolvedTs",
                    "getTableId",
                    "getTableIds",
                    "getAssignedSplits",
                    "getAssignedSplit"
                }) {
            Object value = invoke(state, methodName);
            if (value == null) {
                continue;
            }
            result.put(trimGetter(methodName), summarizeValue(value));
        }
        return result;
    }

    private Map<String, Object> projectOffset(Object offset) {
        if (offset == null) {
            return null;
        }
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("className", offset.getClass().getName());
        Map<?, ?> offsetMap = asMap(invoke(offset, "getOffset"));
        if (offsetMap != null) {
            Map<String, Object> values = new LinkedHashMap<>();
            for (Map.Entry<?, ?> entry : offsetMap.entrySet()) {
                values.put(String.valueOf(entry.getKey()), entry.getValue());
            }
            result.put("values", values);
        } else {
            result.put("value", String.valueOf(offset));
        }
        return result;
    }

    private Map<String, Object> baseProjection(Object state) {
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("className", state == null ? null : state.getClass().getName());
        return result;
    }

    private Object summarizeValue(Object value) {
        if (value == null) {
            return null;
        }
        if (value instanceof String || value instanceof Number || value instanceof Boolean) {
            return value;
        }
        if (value instanceof Map) {
            return "map(" + ((Map<?, ?>) value).size() + ")";
        }
        if (value instanceof Collection) {
            return "collection(" + ((Collection<?>) value).size() + ")";
        }
        if (value.getClass().isArray()) {
            return "array(" + Array.getLength(value) + ")";
        }
        return String.valueOf(value);
    }

    private String resolveSubtaskClass(List<List<Object>> splitObjects, String fallbackClassName) {
        for (List<Object> subtaskSplits : splitObjects) {
            for (Object split : subtaskSplits) {
                if (split != null) {
                    return split.getClass().getName();
                }
            }
        }
        return fallbackClassName;
    }

    private String classNameOrDefault(Object value, String fallback) {
        return value == null ? fallback : value.getClass().getName();
    }

    private Map<String, Object> buildUnsupported(
            CheckpointSourceActionMatcher.SourceTarget target, String code, String reason) {
        Map<String, Object> item = new LinkedHashMap<>();
        item.put("configIndex", target.getConfigIndex());
        item.put("pluginName", target.getPluginName());
        item.put("actionName", target.getActionName());
        item.put("code", code);
        item.put("reason", reason);
        return item;
    }

    private Map<String, Object> findActionEntry(Map<?, ?> actionStates, String actionName) {
        if (actionStates == null) {
            return null;
        }
        String expectedSuffix = "[" + actionName + "]";
        for (Map.Entry<?, ?> entry : actionStates.entrySet()) {
            String candidate = String.valueOf(invoke(entry.getKey(), "getName"));
            if (actionName.equals(candidate) || candidate.endsWith(expectedSuffix)) {
                Map<String, Object> result = new LinkedHashMap<>();
                result.put("key", entry.getKey());
                result.put("actionState", entry.getValue());
                return result;
            }
        }
        return null;
    }

    private Object deserializeCompletedCheckpoint(byte[] bytes, ClassLoader classLoader)
            throws IOException {
        try {
            Class<?> clazz = Class.forName(COMPLETED_CHECKPOINT_CLASS, true, classLoader);
            return serializer.deserialize(bytes, clazz);
        } catch (ClassNotFoundException e) {
            throw new ProxyException(500, "CompletedCheckpoint class is unavailable", e);
        }
    }

    private byte[] loadRawBytes(Map<String, Object> request) throws IOException {
        String inlineContent = ProxyRequestUtils.getOptionalString(request, "contentBase64");
        if (inlineContent != null && !inlineContent.trim().isEmpty()) {
            return decodeBase64(inlineContent);
        }

        Map<String, String> config =
                ProxyRequestUtils.toStringMap(ProxyRequestUtils.getMap(request, "config"));
        String storageType = lower(config.get("storage.type"));
        if (storageType == null) {
            throw new ProxyException(
                    400, "Checkpoint source state inspect requires config.storage.type");
        }
        String namespace = config.get("namespace");
        if (namespace == null || namespace.trim().isEmpty()) {
            throw new ProxyException(
                    400, "Checkpoint source state inspect requires config.namespace");
        }
        String pathValue = ProxyRequestUtils.getOptionalString(request, "path");
        if (pathValue == null || pathValue.trim().isEmpty()) {
            throw new ProxyException(400, "Checkpoint source state inspect requires path");
        }
        return readWithFileSystem(config, storageType, namespace, pathValue);
    }

    private byte[] readWithFileSystem(
            Map<String, String> config, String storageType, String namespace, String requestedPath)
            throws IOException {
        Configuration conf = new Configuration(false);
        for (Map.Entry<String, String> entry : config.entrySet()) {
            String key = entry.getKey();
            String value = entry.getValue();
            if (value == null || value.trim().isEmpty()) {
                continue;
            }
            conf.set(key, value);
            if (key.startsWith("seatunnel.hadoop.")) {
                conf.set(key.substring("seatunnel.hadoop.".length()), value);
            }
        }
        Path targetPath = buildTargetPath(config, storageType, namespace, requestedPath);
        FileSystem fs = targetPath.getFileSystem(conf);
        if (!fs.exists(targetPath)) {
            throw new ProxyException(404, "Checkpoint file does not exist: " + targetPath);
        }
        FileStatus status = fs.getFileStatus(targetPath);
        if (status.isDirectory()) {
            throw new ProxyException(
                    400, "Checkpoint inspect-source-state requires a file path: " + targetPath);
        }
        try (FSDataInputStream in = fs.open(targetPath)) {
            return readAll(in);
        }
    }

    private byte[] readAll(FSDataInputStream in) throws IOException {
        byte[] buffer = new byte[4096];
        int bytesRead;
        try (FSDataInputStream stream = in;
                ByteArrayOutputStream out = new ByteArrayOutputStream()) {
            while ((bytesRead = stream.read(buffer)) >= 0) {
                out.write(buffer, 0, bytesRead);
            }
            return out.toByteArray();
        }
    }

    private byte[] decodeBase64(String content) {
        try {
            return Base64.getDecoder().decode(content);
        } catch (IllegalArgumentException e) {
            throw new ProxyException(400, "Invalid base64 content", e);
        }
    }

    private Path buildTargetPath(
            Map<String, String> config,
            String storageType,
            String namespace,
            String requestedPath) {
        String effectivePath = firstNonBlank(requestedPath, namespace);
        String normalizedPath = effectivePath == null ? namespace.trim() : effectivePath.trim();
        if ("s3".equals(storageType)) {
            String bucket = firstNonBlank(config.get("s3.bucket"), config.get("fs.defaultFS"));
            if (bucket == null) {
                throw new ProxyException(
                        400, "S3 checkpoint inspect requires s3.bucket or fs.defaultFS");
            }
            return new Path(
                    joinBucketAndNamespace(normalizeBucket(bucket, "s3a://"), normalizedPath));
        }
        if ("oss".equals(storageType)) {
            String bucket = firstNonBlank(config.get("oss.bucket"), config.get("fs.defaultFS"));
            if (bucket == null) {
                throw new ProxyException(
                        400, "OSS checkpoint inspect requires oss.bucket or fs.defaultFS");
            }
            return new Path(
                    joinBucketAndNamespace(normalizeBucket(bucket, "oss://"), normalizedPath));
        }
        String defaultFS =
                firstNonBlank(
                        config.get("fs.defaultFS"), config.get("seatunnel.hadoop.fs.defaultFS"));
        if (defaultFS != null) {
            return new Path(new Path(defaultFS), normalizedPath);
        }
        return new Path(normalizedPath);
    }

    private String normalizeBucket(String bucket, String scheme) {
        String trimmed = bucket.trim();
        if (trimmed.contains("://")) {
            return trimmed;
        }
        return scheme + trimmed.replaceFirst("^/+", "");
    }

    private String joinBucketAndNamespace(String bucket, String namespace) {
        String normalizedBucket = bucket.replaceAll("/+$", "");
        String normalizedNamespace = namespace.replaceFirst("^/+", "");
        if (normalizedNamespace.isEmpty()) {
            return normalizedBucket;
        }
        return normalizedBucket + "/" + normalizedNamespace;
    }

    private String firstNonBlank(String... values) {
        for (String value : values) {
            if (value != null && !value.trim().isEmpty()) {
                return value.trim();
            }
        }
        return null;
    }

    private String lower(String value) {
        if (value == null) {
            return null;
        }
        return value.trim().toLowerCase(Locale.ROOT);
    }

    private Map<String, Object> buildPipelineState(PipelineState state) {
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("jobId", state.getJobId());
        result.put("pipelineId", state.getPipelineId());
        result.put("checkpointId", state.getCheckpointId());
        result.put("stateBytes", state.getStates() == null ? 0 : state.getStates().length);
        return result;
    }

    private Map<String, Object> buildCompletedCheckpoint(Object checkpoint) {
        Map<String, Object> result = new LinkedHashMap<>();
        result.put("jobId", invoke(checkpoint, "getJobId"));
        result.put("pipelineId", invoke(checkpoint, "getPipelineId"));
        result.put("checkpointId", invoke(checkpoint, "getCheckpointId"));
        Object checkpointType = invoke(checkpoint, "getCheckpointType");
        result.put(
                "checkpointType", checkpointType == null ? null : String.valueOf(checkpointType));
        result.put("triggerTimestamp", invoke(checkpoint, "getCheckpointTimestamp"));
        result.put("completedTimestamp", invoke(checkpoint, "getCompletedTimestamp"));
        result.put("restored", invoke(checkpoint, "isRestored"));
        result.put("taskStateCount", sizeOfMap(asMap(invoke(checkpoint, "getTaskStates"))));
        result.put(
                "taskStatisticsCount", sizeOfMap(asMap(invoke(checkpoint, "getTaskStatistics"))));
        return result;
    }

    private byte[] firstStateChunk(Object state) {
        List<byte[]> chunks = copyStateChunks(state);
        return chunks.isEmpty() ? null : chunks.get(0);
    }

    private List<byte[]> copyStateChunks(Object state) {
        List<?> chunks = asList(invoke(state, "getState"));
        if (chunks == null || chunks.isEmpty()) {
            return Collections.emptyList();
        }
        List<byte[]> results = new ArrayList<>(chunks.size());
        for (Object chunk : chunks) {
            if (chunk instanceof byte[]) {
                results.add((byte[]) chunk);
            }
        }
        return results;
    }

    private int stateChunkCount(Object state) {
        List<?> chunks = asList(invoke(state, "getState"));
        return chunks == null ? 0 : chunks.size();
    }

    private long stateChunkBytes(Object state) {
        List<?> chunks = asList(invoke(state, "getState"));
        if (chunks == null) {
            return 0L;
        }
        long total = 0L;
        for (Object chunk : chunks) {
            if (chunk instanceof byte[]) {
                total += ((byte[]) chunk).length;
            }
        }
        return total;
    }

    private int sizeOfMap(Map<?, ?> map) {
        return map == null ? 0 : map.size();
    }

    private int sizeOfCollection(Collection<?> items) {
        return items == null ? 0 : items.size();
    }

    private List<String> toStringList(Collection<?> values) {
        if (values == null || values.isEmpty()) {
            return Collections.emptyList();
        }
        List<String> results = new ArrayList<>(values.size());
        for (Object value : values) {
            results.add(String.valueOf(value));
        }
        return results;
    }

    @SuppressWarnings("unchecked")
    private Map<?, ?> asMap(Object value) {
        return value instanceof Map ? (Map<?, ?>) value : null;
    }

    @SuppressWarnings("unchecked")
    private List<?> asList(Object value) {
        return value instanceof List ? (List<?>) value : null;
    }

    @SuppressWarnings("unchecked")
    private Collection<?> asCollection(Object value) {
        return value instanceof Collection ? (Collection<?>) value : null;
    }

    private byte[] asByteArray(Object value) {
        return value instanceof byte[] ? (byte[]) value : null;
    }

    private String trimGetter(String methodName) {
        if (methodName.startsWith("get") && methodName.length() > 3) {
            return Character.toLowerCase(methodName.charAt(3)) + methodName.substring(4);
        }
        return methodName;
    }

    private Object invoke(Object target, String methodName) {
        if (target == null) {
            return null;
        }
        try {
            Method method = target.getClass().getMethod(methodName);
            return method.invoke(target);
        } catch (Exception e) {
            return null;
        }
    }
}
