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

import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Map;

/**
 * Normalized representation of a completed checkpoint, abstracting away differences between
 * SeaTunnel 2.3.x and 3.0.x structures (incremental states, savepoint flags, etc).
 */
public final class CompletedCheckpointData {

    private final Object completedCheckpoint;
    private final PipelineState pipelineState;
    private final Map<String, Object> actionStates;
    private final Map<String, Object> taskStatistics;
    private final boolean incremental;
    private final boolean savepoint;
    private final String engineVersion;
    private final Map<String, Object> extraMetadata;

    public CompletedCheckpointData(
            Object completedCheckpoint,
            PipelineState pipelineState,
            Map<String, Object> actionStates,
            Map<String, Object> taskStatistics,
            boolean incremental,
            boolean savepoint,
            String engineVersion,
            Map<String, Object> extraMetadata) {
        this.completedCheckpoint = completedCheckpoint;
        this.pipelineState = pipelineState;
        this.actionStates = actionStates != null ? actionStates : Collections.emptyMap();
        this.taskStatistics = taskStatistics != null ? taskStatistics : Collections.emptyMap();
        this.incremental = incremental;
        this.savepoint = savepoint;
        this.engineVersion = engineVersion;
        this.extraMetadata = extraMetadata != null ? extraMetadata : new LinkedHashMap<>();
    }

    public Object getCompletedCheckpoint() {
        return completedCheckpoint;
    }

    public PipelineState getPipelineState() {
        return pipelineState;
    }

    public Map<String, Object> getActionStates() {
        return actionStates;
    }

    public Map<String, Object> getTaskStatistics() {
        return taskStatistics;
    }

    public boolean isIncremental() {
        return incremental;
    }

    public boolean isSavepoint() {
        return savepoint;
    }

    public String getEngineVersion() {
        return engineVersion;
    }

    public Map<String, Object> getExtraMetadata() {
        return extraMetadata;
    }
}
