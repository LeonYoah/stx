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
import java.util.List;

public class DagParseResult {

    private final boolean ok;
    private final boolean simpleGraph;
    private final int sourceCount;
    private final int transformCount;
    private final int sinkCount;
    private final List<String> warnings;
    private final DatasetDag graph;

    public DagParseResult(
            boolean ok,
            boolean simpleGraph,
            int sourceCount,
            int transformCount,
            int sinkCount,
            List<String> warnings,
            DatasetDag graph) {
        this.ok = ok;
        this.simpleGraph = simpleGraph;
        this.sourceCount = sourceCount;
        this.transformCount = transformCount;
        this.sinkCount = sinkCount;
        this.warnings = Collections.unmodifiableList(warnings);
        this.graph = graph;
    }

    public boolean isOk() {
        return ok;
    }

    public boolean isSimpleGraph() {
        return simpleGraph;
    }

    public int getSourceCount() {
        return sourceCount;
    }

    public int getTransformCount() {
        return transformCount;
    }

    public int getSinkCount() {
        return sinkCount;
    }

    public List<String> getWarnings() {
        return warnings;
    }

    public DatasetDag getGraph() {
        return graph;
    }
}
