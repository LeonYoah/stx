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

package discovery

import (
	"path/filepath"
	"strings"
	"time"
)

// BuildClustersFromProcesses 按安装目录把进程发现结果聚合成集群列表（主机「发现集群」用）。
// BuildClustersFromProcesses groups process discovery results into clusters by install directory
// (used by host 「发现集群」).
func BuildClustersFromProcesses(processes []*DiscoveredProcess) []*DiscoveredCluster {
	byDir := make(map[string]*DiscoveredCluster)
	order := make([]string, 0)
	now := time.Now()

	for _, p := range processes {
		if p == nil {
			continue
		}
		installDir := strings.TrimSpace(p.InstallDir)
		if installDir == "" {
			continue
		}
		c, ok := byDir[installDir]
		if !ok {
			name := strings.TrimSpace(p.ClusterName)
			if name == "" {
				name = filepath.Base(strings.TrimRight(installDir, "/"))
			}
			if name == "" || name == "." || name == "/" {
				name = "discovered"
			}
			c = &DiscoveredCluster{
				Name:         name,
				InstallDir:   installDir,
				Version:      strings.TrimSpace(p.Version),
				Nodes:        make([]*DiscoveredNode, 0, 1),
				DiscoveredAt: now,
				IsNew:        true,
			}
			byDir[installDir] = c
			order = append(order, installDir)
		}
		if c.Version == "" && strings.TrimSpace(p.Version) != "" {
			c.Version = strings.TrimSpace(p.Version)
		}
		if strings.TrimSpace(c.Name) == "" || c.Name == "discovered" {
			if n := strings.TrimSpace(p.ClusterName); n != "" {
				c.Name = n
			}
		}
		c.Nodes = append(c.Nodes, &DiscoveredNode{
			PID:           p.PID,
			Role:          strings.TrimSpace(p.Role),
			HazelcastPort: p.HazelcastPort,
			APIPort:       p.APIPort,
			JavaHome:      strings.TrimSpace(p.JavaHome),
		})
	}

	out := make([]*DiscoveredCluster, 0, len(order))
	for _, dir := range order {
		c := byDir[dir]
		c.DeploymentMode = inferDeploymentModeFromNodes(c.Nodes)
		out = append(out, c)
	}
	return out
}

func inferDeploymentModeFromNodes(nodes []*DiscoveredNode) string {
	hasMaster, hasWorker, hasHybrid := false, false, false
	for _, n := range nodes {
		if n == nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(n.Role)) {
		case "master":
			hasMaster = true
		case "worker":
			hasWorker = true
		case "hybrid", "master/worker", "master_worker":
			hasHybrid = true
		}
	}
	if hasMaster && hasWorker && !hasHybrid {
		return "separated"
	}
	return "hybrid"
}
