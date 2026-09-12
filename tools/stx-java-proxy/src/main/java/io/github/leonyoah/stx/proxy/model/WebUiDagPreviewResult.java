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

public class WebUiDagPreviewResult {

    private final String jobId;
    private final String jobName;
    private final String jobStatus;
    private final String errorMsg;
    private final String createTime;
    private final String finishTime;
    private final WebUiJobDag jobDag;
    private final Map<String, Object> metrics;
    private final List<String> pluginJarsUrls;
    private final boolean simpleGraph;
    private final List<String> warnings;

    public WebUiDagPreviewResult(
            String jobId,
            String jobName,
            String jobStatus,
            String errorMsg,
            String createTime,
            String finishTime,
            WebUiJobDag jobDag,
            Map<String, Object> metrics,
            List<String> pluginJarsUrls,
            boolean simpleGraph,
            List<String> warnings) {
        this.jobId = jobId;
        this.jobName = jobName;
        this.jobStatus = jobStatus;
        this.errorMsg = errorMsg;
        this.createTime = createTime;
        this.finishTime = finishTime;
        this.jobDag = jobDag;
        this.metrics = Collections.unmodifiableMap(new LinkedHashMap<>(metrics));
        this.pluginJarsUrls = Collections.unmodifiableList(pluginJarsUrls);
        this.simpleGraph = simpleGraph;
        this.warnings = Collections.unmodifiableList(warnings);
    }

    public String getJobId() {
        return jobId;
    }

    public String getJobName() {
        return jobName;
    }

    public String getJobStatus() {
        return jobStatus;
    }

    public String getErrorMsg() {
        return errorMsg;
    }

    public String getCreateTime() {
        return createTime;
    }

    public String getFinishTime() {
        return finishTime;
    }

    public WebUiJobDag getJobDag() {
        return jobDag;
    }

    public Map<String, Object> getMetrics() {
        return metrics;
    }

    public List<String> getPluginJarsUrls() {
        return pluginJarsUrls;
    }

    public boolean isSimpleGraph() {
        return simpleGraph;
    }

    public List<String> getWarnings() {
        return warnings;
    }
}
