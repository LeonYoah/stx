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

	"github.com/LeonYoah/stx/internal/apps/javaruntime"
)

// NodeJavaRuntime 是节点实际生效的 JDK：配置值来自 cluster_nodes.java_home，路径与版本由 Agent 现场检查得出（AC7）。
// NodeJavaRuntime is the node's effective JDK: the configured value comes from cluster_nodes.java_home, and the
// path and version are checked live by the Agent (AC7).
type NodeJavaRuntime struct {
	NodeID     uint   `json:"node_id"`
	HostID     uint   `json:"host_id"`
	Configured string `json:"configured_java_home"`
	javaruntime.Result
}

// GetNodeJavaRuntime 读取节点生效的 JDK：按节点记录的 java_home 让 Agent 检查 <java_home>/bin/java，
// 未指定时返回 Agent 自动探测到的 JDK。Agent 不在线时返回错误，不猜测结果。
// GetNodeJavaRuntime reads the node's effective JDK: the Agent checks <java_home>/bin/java from the node record,
// or reports its auto-detected JDK when none is set. An offline Agent returns an error instead of a guess.
func (s *Service) GetNodeJavaRuntime(ctx context.Context, clusterID, nodeID uint) (*NodeJavaRuntime, error) {
	node, err := s.repo.GetNodeByID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if node == nil || node.ClusterID != clusterID {
		return nil, ErrNodeNotFound
	}
	if s.hostProvider == nil {
		return nil, fmt.Errorf("host provider not configured")
	}
	hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
	if err != nil {
		return nil, err
	}
	if hostInfo == nil || !hostInfo.IsOnline(s.heartbeatTimeout) {
		return nil, fmt.Errorf("host is offline")
	}
	if s.agentSender == nil || hostInfo.AgentID == "" {
		return nil, fmt.Errorf("agent sender not configured")
	}
	// JDK 判定按该节点所属集群的 SeaTunnel 版本分档（读取失败时回退到版本未知，不阻断详情展示）。
	// The JDK verdict follows the cluster's SeaTunnel version (a read failure falls back to unknown instead of
	// breaking the detail view).
	clusterVersion, verErr := s.GetClusterVersion(ctx, node.ClusterID)
	if verErr != nil {
		clusterVersion = ""
	}
	configured := javaruntime.Normalize(node.JavaHome)
	success, output, err := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "check_java", javaruntime.CheckParams(configured))
	if err != nil {
		return nil, fmt.Errorf("check node JDK: %w", err)
	}
	result := javaruntime.Interpret(configured, success, output)
	// Interpret 已判失败的结果（如旧 Agent 忽略自定义 java_home）不能被版本矩阵翻案，
	// 只在成功且未判失败时按集群版本重新分档（3.1.0 集群上的 Java 8 判为不可用）。
	// A result Interpret already failed (e.g. an old Agent ignoring a custom java_home) must not be
	// overturned by the version matrix; re-tier only on success (Java 8 fails on a 3.1.0 cluster).
	if success && result.Status != javaruntime.StatusFailed {
		result.Status = javaruntime.StatusForTarget(result.Version, clusterVersion)
		if result.Status == javaruntime.StatusFailed && result.Version > 0 {
			if result.Version > javaruntime.MaxSupportedMajor(clusterVersion) {
				// 高于支持上限（JDK 24+，JEP 486）是另一方向的不可用，不能与低于下限共用文案。
				// Above the supported ceiling (JDK 24+, JEP 486) is unusable in the other direction
				// and cannot share the "below minimum" wording.
				result.Reason = "above_supported_java"
				result.Message = fmt.Sprintf(
					"Java %d 超过最高支持版本 %d（JDK 24+ 移除 Subject.getSubject / JEP 486，SeaTunnel 无法启动）/ Java %d exceeds the maximum supported Java %d (JDK 24+ removed Subject.getSubject / JEP 486; SeaTunnel cannot start)",
					result.Version, javaruntime.MaxSupportedMajor(clusterVersion),
					result.Version, javaruntime.MaxSupportedMajor(clusterVersion))
			} else {
				result.Reason = "below_required_java"
				result.Message = fmt.Sprintf(
					"Java %d 低于集群 SeaTunnel %s 要求的 Java %d+ / Java %d is below the Java %d+ required by this cluster's SeaTunnel %s",
					result.Version, clusterVersion, javaruntime.MinSupportedMajor(clusterVersion),
					result.Version, javaruntime.MinSupportedMajor(clusterVersion), clusterVersion)
			}
		}
	}
	return &NodeJavaRuntime{NodeID: node.ID, HostID: node.HostID, Configured: configured, Result: *result}, nil
}
