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

public class WebUiJobDag {

    private final String jobId;
    private final Map<Integer, List<WebUiDagEdge>> pipelineEdges;
    private final Map<Integer, WebUiDagVertexInfo> vertexInfoMap;
    private final Map<String, Object> envOptions;

    public WebUiJobDag(
            String jobId,
            Map<Integer, List<WebUiDagEdge>> pipelineEdges,
            Map<Integer, WebUiDagVertexInfo> vertexInfoMap,
            Map<String, Object> envOptions) {
        this.jobId = jobId;
        this.pipelineEdges = Collections.unmodifiableMap(new LinkedHashMap<>(pipelineEdges));
        this.vertexInfoMap = Collections.unmodifiableMap(new LinkedHashMap<>(vertexInfoMap));
        this.envOptions = Collections.unmodifiableMap(new LinkedHashMap<>(envOptions));
    }

    public String getJobId() {
        return jobId;
    }

    public Map<Integer, List<WebUiDagEdge>> getPipelineEdges() {
        return pipelineEdges;
    }

    public Map<Integer, WebUiDagVertexInfo> getVertexInfoMap() {
        return vertexInfoMap;
    }

    public Map<String, Object> getEnvOptions() {
        return envOptions;
    }
}
