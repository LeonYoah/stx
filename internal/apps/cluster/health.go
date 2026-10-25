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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// memberProbeTTL 控制成员探测结果的缓存时间，避免每次状态查询都打到所有 Agent。
// memberProbeTTL controls how long member probe results are cached so status reads do not hit every Agent.
const memberProbeTTL = 10 * time.Second

type memberProbeResult struct {
	at    time.Time
	known bool
	count int
}

// memberProbeCache 按节点缓存 Hazelcast 成员探测结果。
// memberProbeCache caches Hazelcast member probe results per node.
type memberProbeCache struct {
	mu      sync.Mutex
	entries map[uint]memberProbeResult
}

func (c *memberProbeCache) get(nodeID uint, now time.Time) (memberProbeResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[nodeID]
	if !ok || now.Sub(entry.at) > memberProbeTTL {
		return memberProbeResult{}, false
	}
	return entry, true
}

func (c *memberProbeCache) put(nodeID uint, result memberProbeResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[uint]memberProbeResult)
	}
	c.entries[nodeID] = result
}

// clusterExpectsRunning 判断集群当前是否应处于运行态（停止/新建的集群不要求进程运行）。
// clusterExpectsRunning reports whether the cluster is expected to be running
// (stopped or newly created clusters do not require running processes).
func clusterExpectsRunning(status ClusterStatus) bool {
	switch status {
	case ClusterStatusRunning, ClusterStatusDeploying, ClusterStatusError:
		return true
	default:
		return false
	}
}

// evaluateClusterHealth 根据主机在线、进程运行与成员一致性计算集群健康状态。
// evaluateClusterHealth computes cluster health from host liveness, process liveness and member agreement.
func (s *Service) evaluateClusterHealth(ctx context.Context, cluster *Cluster, statusInfo *ClusterStatusInfo) {
	statusInfo.HealthReasons = nil
	if statusInfo.TotalNodes == 0 {
		statusInfo.HealthStatus = HealthStatusUnknown
		return
	}

	reasons := make([]string, 0)
	for _, node := range statusInfo.Nodes {
		if node == nil {
			continue
		}
		if !node.IsOnline {
			reasons = append(reasons, fmt.Sprintf("node %s is offline / 节点 %s 离线", nodeLabel(node), nodeLabel(node)))
		}
	}

	expectRunning := cluster != nil && clusterExpectsRunning(cluster.Status)
	runningNodes := make([]*NodeStatusInfo, 0, len(statusInfo.Nodes))
	for _, node := range statusInfo.Nodes {
		if node == nil || !node.IsOnline {
			continue
		}
		if node.Status == NodeStatusRunning {
			runningNodes = append(runningNodes, node)
			continue
		}
		if expectRunning {
			reasons = append(reasons, fmt.Sprintf("SeaTunnel process on node %s is not running (status: %s) / 节点 %s 的 SeaTunnel 进程未运行（状态：%s）", nodeLabel(node), node.Status, nodeLabel(node), node.Status))
		}
	}
	statusInfo.RunningNodes = len(runningNodes)

	if expectRunning && len(runningNodes) >= 2 {
		reasons = append(reasons, s.checkMemberAgreement(ctx, cluster, runningNodes)...)
	}

	statusInfo.HealthReasons = reasons
	if len(reasons) > 0 {
		statusInfo.HealthStatus = HealthStatusUnhealthy
		return
	}
	statusInfo.HealthStatus = HealthStatusHealthy
}

// checkMemberAgreement 询问每个运行中节点看到的 Hazelcast 成员数，少于运行节点数即视为成员缺失/分裂。
// 成员数未知（旧 Agent 不支持、REST 不可达）时不判定为不健康，避免误报。
// checkMemberAgreement asks each running node how many Hazelcast members it sees; seeing fewer than the
// number of running nodes means members are missing or the cluster is split. Unknown counts (old Agents,
// unreachable REST) never mark the cluster unhealthy to avoid false alarms.
func (s *Service) checkMemberAgreement(ctx context.Context, cluster *Cluster, runningNodes []*NodeStatusInfo) []string {
	if s.agentSender == nil || s.hostProvider == nil || cluster == nil {
		return nil
	}
	nodesByID := make(map[uint]*ClusterNode, len(cluster.Nodes))
	for i := range cluster.Nodes {
		nodesByID[cluster.Nodes[i].ID] = &cluster.Nodes[i]
	}
	expected := len(runningNodes)
	reasons := make([]string, 0)
	for _, info := range runningNodes {
		node := nodesByID[info.NodeID]
		if node == nil {
			continue
		}
		probe := s.probeNodeMembers(ctx, node, info.HostIP)
		if !probe.known {
			continue
		}
		count := probe.count
		info.MemberCount = &count
		if count < expected {
			reasons = append(reasons, fmt.Sprintf("node %s sees only %d/%d cluster members (possible split brain) / 节点 %s 只看到 %d/%d 个集群成员（可能脑裂）", nodeLabel(info), count, expected, nodeLabel(info), count, expected))
		}
	}
	return reasons
}

func (s *Service) probeNodeMembers(ctx context.Context, node *ClusterNode, hostIP string) memberProbeResult {
	now := time.Now()
	if cached, ok := s.memberProbes.get(node.ID, now); ok {
		return cached
	}
	result := memberProbeResult{at: now}
	port := node.HazelcastPort
	if port <= 0 {
		port = node.WorkerPort
	}
	if port <= 0 {
		s.memberProbes.put(node.ID, result)
		return result
	}
	hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
	if err != nil || hostInfo == nil || hostInfo.AgentID == "" {
		s.memberProbes.put(node.ID, result)
		return result
	}
	params := map[string]string{
		"port": strconv.Itoa(port),
		"host": hostIP,
	}
	success, message, err := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "check_cluster_members", params)
	if err == nil && success {
		if count, ok := parseMemberCount(message); ok {
			result.known = true
			result.count = count
		}
	}
	s.memberProbes.put(node.ID, result)
	return result
}

// parseMemberCount 从 Agent 预检结果 JSON 的 details.member_count 中解析成员数。
// parseMemberCount parses details.member_count from an Agent precheck JSON result.
func parseMemberCount(message string) (int, bool) {
	var payload struct {
		Details map[string]string `json:"details"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(message)), &payload); err != nil {
		return 0, false
	}
	raw, ok := payload.Details["member_count"]
	if !ok {
		return 0, false
	}
	count, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || count <= 0 {
		return 0, false
	}
	return count, true
}

func nodeLabel(node *NodeStatusInfo) string {
	if node == nil {
		return ""
	}
	if strings.TrimSpace(node.HostName) != "" {
		return node.HostName
	}
	if strings.TrimSpace(node.HostIP) != "" {
		return node.HostIP
	}
	return fmt.Sprintf("#%d", node.NodeID)
}
