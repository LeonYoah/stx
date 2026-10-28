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

/**
 * dagre 布局单独成文件，保证 dagre 只进入按需加载的拓扑 chunk。
 * The dagre layout lives in its own file so dagre only lands in the lazily loaded topology chunk.
 */

import dagre from '@dagrejs/dagre';
import type {DagGraph} from './dag-model';

export interface NodeSize {
  width: number;
  height: number;
}

// dagre 参数照抄 Flink web-dashboard components/dagre/graph.ts。
// dagre settings copied from Flink web-dashboard components/dagre/graph.ts.
const FLINK_LAYOUT = {
  rankdir: 'LR' as const,
  ranker: 'network-simplex' as const,
  align: 'DL' as const,
  marginx: 20,
  marginy: 20,
  edgesep: 150,
  ranksep: 150,
};

/**
 * 用 dagre 计算左→右布局，返回节点左上角坐标（React Flow 的定位基准）。节点尺寸固定，布局结果只取决于结构。
 * Compute a left-to-right dagre layout and return top-left positions (React Flow's anchor). Node sizes are fixed so
 * the result depends only on the structure.
 */
export function layoutDag(graph: DagGraph, size: NodeSize): Map<number, {x: number; y: number}> {
  const g = new dagre.graphlib.Graph();
  g.setGraph({...FLINK_LAYOUT, nodesep: 60});
  g.setDefaultEdgeLabel(() => ({}));
  for (const vertex of graph.vertices) {
    g.setNode(String(vertex.vertexId), {width: size.width, height: size.height});
  }
  for (const edge of graph.edges) {
    g.setEdge(String(edge.source), String(edge.target));
  }
  dagre.layout(g);
  const positions = new Map<number, {x: number; y: number}>();
  for (const vertex of graph.vertices) {
    const node = g.node(String(vertex.vertexId));
    if (node) {
      positions.set(vertex.vertexId, {x: node.x - size.width / 2, y: node.y - size.height / 2});
    }
  }
  return positions;
}

