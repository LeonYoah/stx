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
	"strings"
	"testing"
)

type healthHostProvider struct{}

func (healthHostProvider) GetHostByID(_ context.Context, id uint) (*HostInfo, error) {
	return &HostInfo{ID: id, AgentID: fmt.Sprintf("agent-%d", id)}, nil
}

// healthMemberSender 按 Agent 返回其看到的成员数；0 表示不支持（旧 Agent）。
// healthMemberSender returns the member count each Agent sees; 0 means unsupported (old Agent).
type healthMemberSender struct {
	counts map[string]int
	calls  int
}

func (s *healthMemberSender) SendCommand(_ context.Context, agentID, commandType string, _ map[string]string) (bool, string, error) {
	s.calls++
	if commandType != "check_cluster_members" {
		return false, "unexpected", nil
	}
	count := s.counts[agentID]
	if count == 0 {
		return false, `{"success":false,"message":"unsupported sub_command"}`, nil
	}
	return true, fmt.Sprintf(`{"success":true,"details":{"member_count":"%d"}}`, count), nil
}

func healthTestCluster(status ClusterStatus, ids ...uint) (*Cluster, *ClusterStatusInfo) {
	cluster := &Cluster{ID: 1, Status: status}
	info := &ClusterStatusInfo{ClusterID: 1, TotalNodes: len(ids)}
	for _, id := range ids {
		cluster.Nodes = append(cluster.Nodes, ClusterNode{ID: id, HostID: id, HazelcastPort: 5801})
		info.Nodes = append(info.Nodes, &NodeStatusInfo{NodeID: id, HostID: id, HostName: fmt.Sprintf("n%d", id), IsOnline: true, Status: NodeStatusRunning})
	}
	return cluster, info
}

func newHealthTestService(sender AgentCommandSender) *Service {
	s := &Service{hostProvider: healthHostProvider{}}
	if sender != nil {
		s.agentSender = sender
	}
	return s
}

func TestEvaluateClusterHealth_AllRunningAndAgreeingIsHealthy(t *testing.T) {
	sender := &healthMemberSender{counts: map[string]int{"agent-1": 3, "agent-2": 3, "agent-3": 3}}
	cluster, info := healthTestCluster(ClusterStatusRunning, 1, 2, 3)
	newHealthTestService(sender).evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusHealthy || len(info.HealthReasons) != 0 || info.RunningNodes != 3 {
		t.Fatalf("expected healthy, got %s %v running=%d", info.HealthStatus, info.HealthReasons, info.RunningNodes)
	}
	if info.Nodes[0].MemberCount == nil || *info.Nodes[0].MemberCount != 3 {
		t.Fatalf("expected member count to be exposed, got %+v", info.Nodes[0].MemberCount)
	}
}

func TestEvaluateClusterHealth_StoppedProcessOnOnlineHostIsUnhealthy(t *testing.T) {
	cluster, info := healthTestCluster(ClusterStatusRunning, 1, 2, 3)
	info.Nodes[1].Status = NodeStatusStopped
	newHealthTestService(nil).evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusUnhealthy {
		t.Fatalf("expected unhealthy, got %s", info.HealthStatus)
	}
	if len(info.HealthReasons) != 1 || !strings.Contains(info.HealthReasons[0], "n2") {
		t.Fatalf("expected one reason naming n2, got %v", info.HealthReasons)
	}
}

func TestEvaluateClusterHealth_SplitMembersIsUnhealthy(t *testing.T) {
	sender := &healthMemberSender{counts: map[string]int{"agent-1": 2, "agent-2": 2, "agent-3": 1}}
	cluster, info := healthTestCluster(ClusterStatusRunning, 1, 2, 3)
	newHealthTestService(sender).evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusUnhealthy || len(info.HealthReasons) != 3 {
		t.Fatalf("expected 3 split-brain reasons, got %s %v", info.HealthStatus, info.HealthReasons)
	}
}

func TestEvaluateClusterHealth_UnknownMemberCountDoesNotAlarm(t *testing.T) {
	sender := &healthMemberSender{counts: map[string]int{}}
	cluster, info := healthTestCluster(ClusterStatusRunning, 1, 2)
	svc := newHealthTestService(sender)
	svc.evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusHealthy {
		t.Fatalf("unknown member counts must not mark unhealthy, got %s %v", info.HealthStatus, info.HealthReasons)
	}
	// 缓存：TTL 内不重复探测。Cache: no re-probe within the TTL.
	calls := sender.calls
	_, info = healthTestCluster(ClusterStatusRunning, 1, 2)
	svc.evaluateClusterHealth(context.Background(), cluster, info)
	if sender.calls != calls {
		t.Fatalf("expected cached probes, calls %d -> %d", calls, sender.calls)
	}
}

func TestEvaluateClusterHealth_StoppedClusterDoesNotRequireProcesses(t *testing.T) {
	cluster, info := healthTestCluster(ClusterStatusStopped, 1, 2)
	for _, node := range info.Nodes {
		node.Status = NodeStatusStopped
	}
	newHealthTestService(nil).evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusHealthy {
		t.Fatalf("a stopped cluster with online hosts should not be unhealthy, got %s %v", info.HealthStatus, info.HealthReasons)
	}

	info.Nodes[0].IsOnline = false
	newHealthTestService(nil).evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusUnhealthy {
		t.Fatalf("an offline host is always unhealthy, got %s", info.HealthStatus)
	}
}

// healthEngineSender 按 Agent 返回 check_engine_health 的原始 JSON；空串模拟旧 Agent 不支持。
// healthEngineSender returns a check_engine_health JSON payload per agent; an empty string
// simulates an old Agent that does not support the command.
type healthEngineSender struct {
	payloads    map[string]string
	engineCalls int
}

func (s *healthEngineSender) SendCommand(_ context.Context, agentID, commandType string, _ map[string]string) (bool, string, error) {
	if commandType != "check_engine_health" {
		return false, "unexpected", nil
	}
	s.engineCalls++
	payload := s.payloads[agentID]
	if payload == "" {
		return false, `{"success":false,"message":"unsupported sub_command"}`, nil
	}
	return true, payload, nil
}

// TestEvaluateClusterHealth_EngineNotReadyIsUnhealthy 半初始化引擎（known+unhealthy）在
// 单节点集群也要被判为不健康，原因透传。
// TestEvaluateClusterHealth_EngineNotReadyIsUnhealthy: a half-initialized engine (known+unhealthy)
// makes even a single-node cluster unhealthy and propagates the reason.
func TestEvaluateClusterHealth_EngineNotReadyIsUnhealthy(t *testing.T) {
	sender := &healthEngineSender{payloads: map[string]string{
		"agent-1": `{"success":false,"details":{"known":"true","healthy":"false","reason":"coordinatorService is null"}}`,
	}}
	cluster, info := healthTestCluster(ClusterStatusRunning, 1)
	cluster.Nodes[0].Role = NodeRoleMasterWorker
	newHealthTestService(sender).evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusUnhealthy {
		t.Fatalf("expected unhealthy, got %s %v", info.HealthStatus, info.HealthReasons)
	}
	if len(info.HealthReasons) != 1 || !strings.Contains(info.HealthReasons[0], "coordinatorService is null") {
		t.Fatalf("expected engine reason, got %v", info.HealthReasons)
	}
}

// TestEvaluateClusterHealth_EngineHealthyStaysHealthy 引擎就绪（known+healthy）不产生原因。
// TestEvaluateClusterHealth_EngineHealthyStaysHealthy: a ready engine adds no reasons.
func TestEvaluateClusterHealth_EngineHealthyStaysHealthy(t *testing.T) {
	sender := &healthEngineSender{payloads: map[string]string{
		"agent-1": `{"success":true,"details":{"known":"true","healthy":"true"}}`,
	}}
	cluster, info := healthTestCluster(ClusterStatusRunning, 1)
	cluster.Nodes[0].Role = NodeRoleMasterWorker
	newHealthTestService(sender).evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusHealthy || len(info.HealthReasons) != 0 {
		t.Fatalf("expected healthy, got %s %v", info.HealthStatus, info.HealthReasons)
	}
}

// TestEvaluateClusterHealth_UnknownEngineStateDoesNotAlarm 未知状态（旧 Agent、REST 不可达）
// 绝不误报；纯 worker 角色不探测。
// TestEvaluateClusterHealth_UnknownEngineStateDoesNotAlarm: unknown states (old Agent,
// unreachable REST) never alarm; worker-only roles are not probed.
func TestEvaluateClusterHealth_UnknownEngineStateDoesNotAlarm(t *testing.T) {
	sender := &healthEngineSender{payloads: map[string]string{
		"agent-1": `{"success":false,"details":{"known":"false","healthy":"false","reason":"connection refused"}}`,
	}}
	cluster, info := healthTestCluster(ClusterStatusRunning, 1, 2)
	cluster.Nodes[0].Role = NodeRoleMasterWorker
	cluster.Nodes[1].Role = NodeRoleWorker
	svc := newHealthTestService(sender)
	svc.evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusHealthy {
		t.Fatalf("unknown engine state must not mark unhealthy, got %s %v", info.HealthStatus, info.HealthReasons)
	}
	if sender.engineCalls != 1 {
		t.Fatalf("worker-only node must not be probed, got %d calls", sender.engineCalls)
	}
	// 缓存：TTL 内不重复探测。Cache: no re-probe within the TTL.
	_, info = healthTestCluster(ClusterStatusRunning, 1, 2)
	svc.evaluateClusterHealth(context.Background(), cluster, info)
	if sender.engineCalls != 1 {
		t.Fatalf("expected cached probe, calls %d -> %d", 1, sender.engineCalls)
	}
}

func TestParseMemberCount(t *testing.T) {
	if n, ok := parseMemberCount(`{"details":{"member_count":"3"}}`); !ok || n != 3 {
		t.Fatalf("got %d %v", n, ok)
	}
	for _, raw := range []string{"", "not json", `{"details":{}}`, `{"details":{"member_count":"0"}}`} {
		if _, ok := parseMemberCount(raw); ok {
			t.Fatalf("expected unknown for %q", raw)
		}
	}
}
