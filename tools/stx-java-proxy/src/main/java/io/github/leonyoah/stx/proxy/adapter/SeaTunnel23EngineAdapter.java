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

package io.github.leonyoah.stx.proxy.adapter;

import org.apache.seatunnel.engine.checkpoint.storage.PipelineState;

import java.lang.reflect.Method;
import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/** Adapter implementation for SeaTunnel 2.3.x engine runtime. */
public class SeaTunnel23EngineAdapter extends AbstractSeaTunnelEngineAdapter {

    private static final String COMPLETED_CHECKPOINT_CLASS =
            "org.apache.seatunnel.engine.server.checkpoint.CompletedCheckpoint";

    @Override
    public String getAdapterVersion() {
        return EngineAdapterVersionPolicy.ADAPTER_V23;
    }

    @Override
    public boolean supports(EngineVersion version) {
        if (version == null) {
            return false;
        }
        return version.matches("2.3.*") || version.isVersion2x();
    }

    @Override
    public CompletedCheckpointData deserializeCheckpoint(byte[] rawBytes, ClassLoader classLoader)
            throws Exception {
        return runWithClassLoader(
                classLoader,
                () -> {
                    PipelineState pipelineState =
                            protoSerializer.deserialize(rawBytes, PipelineState.class);
                    Class<?> completedCheckpointClazz =
                            Class.forName(COMPLETED_CHECKPOINT_CLASS, true, classLoader);
                    Object completedCheckpoint =
                            protoSerializer.deserialize(
                                    pipelineState.getStates(), completedCheckpointClazz);

                    Map<String, Object> actionStates =
                            extractMapQuietly(completedCheckpoint, "getTaskStates");
                    Map<String, Object> taskStatistics =
                            extractMapQuietly(completedCheckpoint, "getTaskStatistics");

                    boolean savepoint = false;
                    Object checkpointType =
                            readPropertyQuietly(completedCheckpoint, "checkpointType");
                    if (checkpointType != null
                            && checkpointType.toString().toUpperCase().contains("SAVEPOINT")) {
                        savepoint = true;
                    }

                    Map<String, Object> extra = new LinkedHashMap<>();
                    extra.put("engineVersion", "2.3.x");
                    extra.put(
                            "checkpointType",
                            checkpointType != null ? checkpointType.toString() : "CHECKPOINT_TYPE");

                    return new CompletedCheckpointData(
                            completedCheckpoint,
                            pipelineState,
                            actionStates,
                            taskStatistics,
                            false,
                            savepoint,
                            "2.3.x",
                            extra);
                });
    }

    @Override
    public List<Map<String, Object>> inspectSources(
            Map<String, Object> request,
            ClassLoader classLoader,
            CompletedCheckpointData checkpointData,
            List<String> warnings)
            throws Exception {
        return runWithClassLoader(
                classLoader,
                () -> {
                    List<Map<String, Object>> result = new ArrayList<>();
                    // Extract from action states
                    Map<String, Object> actionStates = checkpointData.getActionStates();
                    for (Map.Entry<String, Object> entry : actionStates.entrySet()) {
                        String actionName = entry.getKey();
                        if (actionName != null && actionName.toLowerCase().startsWith("source")) {
                            Map<String, Object> sourceInfo = new LinkedHashMap<>();
                            sourceInfo.put("actionName", actionName);
                            sourceInfo.put("stateData", entry.getValue());
                            result.add(sourceInfo);
                        }
                    }
                    return result;
                });
    }

    @Override
    public List<Map<String, Object>> inspectSinks(
            Map<String, Object> request,
            ClassLoader classLoader,
            CompletedCheckpointData checkpointData)
            throws Exception {
        return runWithClassLoader(
                classLoader,
                () -> {
                    List<Map<String, Object>> sinks = new ArrayList<>();
                    Map<String, Object> actionStates = checkpointData.getActionStates();
                    for (Map.Entry<String, Object> entry : actionStates.entrySet()) {
                        String actionName = entry.getKey();
                        if (actionName != null && actionName.toLowerCase().startsWith("sink")) {
                            Map<String, Object> sinkInfo = new LinkedHashMap<>();
                            sinkInfo.put("actionName", actionName);
                            sinkInfo.put("hasCommitState", entry.getValue() != null);
                            sinks.add(sinkInfo);
                        }
                    }
                    return sinks;
                });
    }

    @Override
    public Map<String, Object> getEngineCapabilities() {
        Map<String, Object> caps = new LinkedHashMap<>();
        caps.put("adapterVersion", getAdapterVersion());
        caps.put("engineFamily", "seatunnel");
        caps.put("incrementalCheckpoint", false);
        caps.put("tableLevelState", false);
        caps.put("savepointSupported", true);
        return caps;
    }

    @SuppressWarnings("unchecked")
    private Map<String, Object> extractMapQuietly(Object target, String methodName) {
        if (target == null) {
            return Collections.emptyMap();
        }
        try {
            Method method = target.getClass().getMethod(methodName);
            Object val = method.invoke(target);
            if (val instanceof Map) {
                return (Map<String, Object>) val;
            }
        } catch (Exception e) {
            log.debug(
                    "Failed to invoke {} on {}: {}",
                    methodName,
                    target.getClass().getName(),
                    e.getMessage());
        }
        return Collections.emptyMap();
    }
}
