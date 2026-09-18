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

import org.apache.seatunnel.shade.com.typesafe.config.Config;

import java.util.Collections;
import java.util.List;

public class JobConfigContext {

    private final Config jobConfig;
    private final List<Config> sources;
    private final List<Config> transforms;
    private final List<Config> sinks;
    private final boolean simpleGraph;
    private final List<String> warnings;
    private final DatasetDag graph;

    public JobConfigContext(
            Config jobConfig,
            List<Config> sources,
            List<Config> transforms,
            List<Config> sinks,
            boolean simpleGraph,
            List<String> warnings,
            DatasetDag graph) {
        this.jobConfig = jobConfig;
        this.sources = Collections.unmodifiableList(sources);
        this.transforms = Collections.unmodifiableList(transforms);
        this.sinks = Collections.unmodifiableList(sinks);
        this.simpleGraph = simpleGraph;
        this.warnings = Collections.unmodifiableList(warnings);
        this.graph = graph;
    }

    public Config getJobConfig() {
        return jobConfig;
    }

    public List<Config> getSources() {
        return sources;
    }

    public List<Config> getTransforms() {
        return transforms;
    }

    public List<Config> getSinks() {
        return sinks;
    }

    public boolean isSimpleGraph() {
        return simpleGraph;
    }

    public List<String> getWarnings() {
        return warnings;
    }

    public DatasetDag getGraph() {
        return graph;
    }
}
