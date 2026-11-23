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

// engineProbeResult 是一次引擎业务就绪探测的结论（check_engine_health）。
// known=false 表示不可判定（旧 Agent、网络隔离、REST 不存在），绝不判为不健康。
// engineProbeResult is one engine readiness probe outcome (check_engine_health).
// known=false means undecidable (old Agent, isolated network, missing REST) and never marks unhealthy.
type engineProbeResult struct {
	at      time.Time
	known   bool
	healthy bool
	reason  string
}

// engineProbeCache 按节点缓存引擎就绪探测结果，复用 memberProbeTTL，避免每次刷新打爆 Agent。
// engineProbeCache caches engine readiness per node, reusing memberProbeTTL so refreshes do not hammer Agents.
type engineProbeCache struct {
	mu      sync.Mutex
	entries map[uint]engineProbeResult
}

func (c *engineProbeCache) get(nodeID uint, now time.Time) (engineProbeResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[nodeID]
	if !ok || now.Sub(entry.at) > memberProbeTTL {
		return engineProbeResult{}, false
	}
	return entry, true
}

func (c *engineProbeCache) put(nodeID uint, result engineProbeResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[uint]engineProbeResult)
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
	statusInfo.HealthReasonDetails = nil
	if statusInfo.TotalNodes == 0 {
		statusInfo.HealthStatus = HealthStatusUnknown
		return
	}

	reasons := make([]string, 0)
	details := make([]ClusterHealthReason, 0)
	for _, node := range statusInfo.Nodes {
		if node == nil {
			continue
		}
		if !node.IsOnline {
			reasons = append(reasons, fmt.Sprintf("node %s is offline / 节点 %s 离线", nodeLabel(node), nodeLabel(node)))
			details = append(details, ClusterHealthReason{Code: ClusterHealthReasonNodeOffline, Node: nodeLabel(node)})
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
			details = append(details, ClusterHealthReason{Code: ClusterHealthReasonProcessNotRunning, Node: nodeLabel(node), Status: string(node.Status)})
		}
	}
	statusInfo.RunningNodes = len(runningNodes)

	if expectRunning && len(runningNodes) >= 2 {
		memberReasons, memberDetails := s.checkMemberAgreement(ctx, cluster, runningNodes)
		reasons = append(reasons, memberReasons...)
		details = append(details, memberDetails...)
	}

	// 进程存活但引擎半初始化（coordinatorService == null 等）时 maps/overview 明确返回失败；
	// 单节点集群也要检测，只有明确判定不健康才计为不健康。
	// A live process can still host a half-initialized engine (e.g. coordinatorService == null) where
	// maps/overview fails explicitly; single-node clusters are probed too, and only a definitive
	// unhealthy answer counts.
	if expectRunning && len(runningNodes) >= 1 {
		engineReasons, engineDetails := s.checkEngineReadiness(ctx, cluster, runningNodes)
		reasons = append(reasons, engineReasons...)
		details = append(details, engineDetails...)
	}

	statusInfo.HealthReasons = reasons
	statusInfo.HealthReasonDetails = details
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
func (s *Service) checkMemberAgreement(ctx context.Context, cluster *Cluster, runningNodes []*NodeStatusInfo) ([]string, []ClusterHealthReason) {
	if s.agentSender == nil || s.hostProvider == nil || cluster == nil {
		return nil, nil
	}
	nodesByID := make(map[uint]*ClusterNode, len(cluster.Nodes))
	for i := range cluster.Nodes {
		nodesByID[cluster.Nodes[i].ID] = &cluster.Nodes[i]
	}
	expected := len(runningNodes)
	reasons := make([]string, 0)
	details := make([]ClusterHealthReason, 0)
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
			details = append(details, ClusterHealthReason{Code: ClusterHealthReasonMemberSplitBrain, Node: nodeLabel(info), ObservedMembers: count, ExpectedMembers: expected})
		}
	}
	return reasons, details
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

// checkEngineReadiness 对运行中的 master / master_worker 节点调用引擎就绪探测；
// 明确回答 known=true 且 unhealthy 时把原因并入健康检查，未知结果一律跳过。
// checkEngineReadiness probes running master / master_worker nodes for engine readiness;
// only a definitive known=true + unhealthy answer adds a health reason, unknown results are skipped.
func (s *Service) checkEngineReadiness(ctx context.Context, cluster *Cluster, runningNodes []*NodeStatusInfo) ([]string, []ClusterHealthReason) {
	if s.agentSender == nil || s.hostProvider == nil || cluster == nil {
		return nil, nil
	}
	nodesByID := make(map[uint]*ClusterNode, len(cluster.Nodes))
	for i := range cluster.Nodes {
		nodesByID[cluster.Nodes[i].ID] = &cluster.Nodes[i]
	}
	reasons := make([]string, 0)
	details := make([]ClusterHealthReason, 0)
	for _, info := range runningNodes {
		node := nodesByID[info.NodeID]
		if node == nil {
			continue
		}
		if node.Role != NodeRoleMaster && node.Role != NodeRoleMasterWorker {
			continue
		}
		probe := s.probeNodeEngine(ctx, node, info.HostIP)
		if !probe.known || probe.healthy {
			continue
		}
		reasons = append(reasons, fmt.Sprintf("SeaTunnel engine on node %s is not ready: %s / 节点 %s 的 SeaTunnel 引擎未就绪：%s",
			nodeLabel(info), probe.reason, nodeLabel(info), probe.reason))
		details = append(details, ClusterHealthReason{Code: ClusterHealthReasonEngineNotReady, Node: nodeLabel(info), Detail: probe.reason})
	}
	return reasons, details
}

func (s *Service) probeNodeEngine(ctx context.Context, node *ClusterNode, hostIP string) engineProbeResult {
	now := time.Now()
	if cached, ok := s.engineProbes.get(node.ID, now); ok {
		return cached
	}
	result := engineProbeResult{at: now}
	port := node.HazelcastPort
	if port <= 0 {
		port = node.WorkerPort
	}
	if port <= 0 {
		s.engineProbes.put(node.ID, result)
		return result
	}
	hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
	if err != nil || hostInfo == nil || hostInfo.AgentID == "" {
		s.engineProbes.put(node.ID, result)
		return result
	}
	_, message, err := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "check_engine_health", map[string]string{
		"port": strconv.Itoa(port),
		"host": hostIP,
	})
	if err == nil {
		result = parseEngineProbe(message, now)
	}
	s.engineProbes.put(node.ID, result)
	return result
}

// parseEngineProbe 解析 check_engine_health 输出中的 known / healthy / reason 字段。
// parseEngineProbe parses known / healthy / reason from a check_engine_health result payload.
func parseEngineProbe(message string, now time.Time) engineProbeResult {
	result := engineProbeResult{at: now}
	var payload struct {
		Details map[string]string `json:"details"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(message)), &payload); err != nil {
		return result
	}
	result.known = payload.Details["known"] == "true"
	result.healthy = payload.Details["healthy"] == "true"
	result.reason = strings.TrimSpace(payload.Details["reason"])
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
