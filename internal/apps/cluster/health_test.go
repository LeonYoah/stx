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

func newHealthTestService(sender *healthMemberSender) *Service {
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
