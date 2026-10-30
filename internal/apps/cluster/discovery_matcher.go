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

package cluster

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/discovery"
	"github.com/LeonYoah/stx/internal/apps/javaruntime"
	"github.com/LeonYoah/stx/internal/logger"
)

// FindClusterByInstallDir 按主机安装目录匹配已纳管集群（discovery.ClusterMatcher）。
// FindClusterByInstallDir matches an already-managed cluster by host install directory (discovery.ClusterMatcher).
func (s *Service) FindClusterByInstallDir(ctx context.Context, hostID uint, installDir string) (clusterID uint, nodeID uint, found bool, err error) {
	if s == nil || s.repo == nil {
		return 0, 0, false, fmt.Errorf("cluster service not configured / 集群服务未配置")
	}
	installDir = strings.TrimSpace(installDir)
	if hostID == 0 || installDir == "" {
		return 0, 0, false, nil
	}
	return s.repo.GetNodeByHostAndInstallDir(ctx, hostID, installDir)
}

// CreateClusterFromDiscovery 从「发现集群」结果创建集群并写入节点 java_home（AC1e）。
// CreateClusterFromDiscovery creates a cluster from a discovery result and persists node java_home (AC1e).
func (s *Service) CreateClusterFromDiscovery(ctx context.Context, hostID uint, dc *discovery.DiscoveredCluster) (uint, error) {
	if s == nil {
		return 0, fmt.Errorf("cluster service not configured / 集群服务未配置")
	}
	if dc == nil {
		return 0, fmt.Errorf("discovered cluster is nil / 发现的集群为空")
	}
	if hostID == 0 {
		return 0, fmt.Errorf("host id is required / 主机 ID 必填")
	}
	installDir := strings.TrimSpace(dc.InstallDir)
	if installDir == "" {
		return 0, fmt.Errorf("install directory is required / 安装目录必填")
	}

	mode := inferDeploymentModeFromDiscovery(dc)
	clusterJavaHome, err := javaruntime.Validate(dc.JavaHome)
	if err != nil {
		return 0, err
	}

	nodes := make([]CreateNodeFromDiscovery, 0, len(dc.Nodes))
	for _, n := range dc.Nodes {
		if n == nil {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(n.Role))
		if role == "" {
			role = "hybrid"
		}
		// 确认请求的集群级 java_home 优先；否则用进程实际 JDK（AC1e / AC1f）。
		// Cluster-level java_home from confirm wins; otherwise use the process JDK (AC1e / AC1f).
		nodeJava := clusterJavaHome
		if nodeJava == "" {
			nodeJava, err = javaruntime.Validate(n.JavaHome)
			if err != nil {
				return 0, err
			}
		}
		nodes = append(nodes, CreateNodeFromDiscovery{
			HostID:        hostID,
			InstallDir:    installDir,
			Role:          role,
			PID:           n.PID,
			HazelcastPort: n.HazelcastPort,
			APIPort:       n.APIPort,
			JavaHome:      nodeJava,
		})
	}
	if len(nodes) == 0 {
		nodes = append(nodes, CreateNodeFromDiscovery{
			HostID:     hostID,
			InstallDir: installDir,
			Role:       discoveryRoleForMode(mode),
			JavaHome:   clusterJavaHome,
		})
	}

	name := strings.TrimSpace(dc.Name)
	if name == "" {
		name = filepath.Base(strings.TrimRight(installDir, "/"))
	}
	if name == "" || name == "." || name == "/" {
		name = fmt.Sprintf("discovered-%d", hostID)
	}

	var cfg ClusterConfig
	if len(dc.Config) > 0 {
		cfg = ClusterConfig(dc.Config)
	}

	created, err := s.Create(ctx, &CreateClusterRequest{
		Name:           name,
		DeploymentMode: mode,
		Version:        strings.TrimSpace(dc.Version),
		InstallDir:     installDir,
		Config:         cfg,
		Nodes:          nodes,
	})
	if err != nil {
		return 0, err
	}
	logger.InfoF(ctx, "[Cluster] Created cluster %d from discovery at %s (java_home=%q) / 从发现结果创建集群 %d（目录 %s，java_home=%q）",
		created.ID, installDir, clusterJavaHome, created.ID, installDir, clusterJavaHome)
	return created.ID, nil
}

// UpdateNodeFromDiscovery 用发现到的进程信息更新已有节点状态（PID / 运行中）。
// UpdateNodeFromDiscovery updates an existing node from a discovered process (PID / running).
func (s *Service) UpdateNodeFromDiscovery(ctx context.Context, nodeID uint, node *discovery.DiscoveredNode) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("cluster service not configured / 集群服务未配置")
	}
	if node == nil || nodeID == 0 {
		return nil
	}
	if node.PID > 0 {
		if err := s.repo.UpdateNodeProcess(ctx, nodeID, node.PID, "running"); err != nil {
			return err
		}
		_ = s.repo.UpdateNodeStatus(ctx, nodeID, NodeStatusRunning)
	}
	// 若节点尚未配置 JDK 且发现到了进程 JDK，补写生效路径（AC1f / AC4c）。
	// If the node has no configured JDK yet and discovery resolved the process JDK, persist it (AC1f / AC4c).
	if strings.TrimSpace(node.JavaHome) != "" {
		existing, err := s.repo.GetNodeByID(ctx, nodeID)
		if err != nil || existing == nil {
			return err
		}
		if strings.TrimSpace(existing.JavaHome) == "" {
			home, vErr := javaruntime.Validate(node.JavaHome)
			if vErr != nil {
				return vErr
			}
			if home != "" {
				existing.JavaHome = home
				return s.repo.UpdateNode(ctx, existing)
			}
		}
	}
	return nil
}

func inferDeploymentModeFromDiscovery(dc *discovery.DiscoveredCluster) DeploymentMode {
	if dc == nil {
		return DeploymentModeHybrid
	}
	switch strings.ToLower(strings.TrimSpace(dc.DeploymentMode)) {
	case "separated":
		return DeploymentModeSeparated
	case "hybrid":
		return DeploymentModeHybrid
	}
	hasMaster, hasWorker := false, false
	for _, n := range dc.Nodes {
		if n == nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(n.Role)) {
		case "master":
			hasMaster = true
		case "worker":
			hasWorker = true
		}
	}
	// 仅当同时看到 master 与 worker 才按分离模式；否则 hybrid，避免 Create 因不完整分离集群失败。
	// Use separated only when both master and worker are present; otherwise hybrid so Create does not reject an incomplete set.
	if hasMaster && hasWorker {
		return DeploymentModeSeparated
	}
	return DeploymentModeHybrid
}

func discoveryRoleForMode(mode DeploymentMode) string {
	if mode == DeploymentModeSeparated {
		return "master"
	}
	return "hybrid"
}
