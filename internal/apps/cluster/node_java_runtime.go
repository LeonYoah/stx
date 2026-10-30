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
		return nil, fmt.Errorf("host provider not configured / 主机提供者未配置")
	}
	hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
	if err != nil {
		return nil, err
	}
	if hostInfo == nil || !hostInfo.IsOnline(s.heartbeatTimeout) {
		return nil, fmt.Errorf("host is offline / 主机离线")
	}
	if s.agentSender == nil || hostInfo.AgentID == "" {
		return nil, fmt.Errorf("agent sender not configured / Agent 发送器未配置")
	}
	configured := javaruntime.Normalize(node.JavaHome)
	success, output, err := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "check_java", javaruntime.CheckParams(configured))
	if err != nil {
		return nil, fmt.Errorf("check node JDK / 检查节点 JDK: %w", err)
	}
	result := javaruntime.Interpret(configured, success, output)
	return &NodeJavaRuntime{NodeID: node.ID, HostID: node.HostID, Configured: configured, Result: *result}, nil
}
