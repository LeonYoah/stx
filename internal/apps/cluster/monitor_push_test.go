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
	"sync"
	"testing"
	"time"
)

// recordingMonitorPusher 线程安全地记录监控配置推送（推送本身是异步的）。
// recordingMonitorPusher records monitor config pushes thread-safely (pushes run asynchronously).
type recordingMonitorPusher struct {
	mu     sync.Mutex
	pushes []string
}

func (m *recordingMonitorPusher) PushClusterMonitorConfigToHost(_ context.Context, clusterID, hostID uint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pushes = append(m.pushes, fmt.Sprintf("%d/%d", clusterID, hostID))
	return nil
}

func (m *recordingMonitorPusher) waitFor(t *testing.T, want int) []string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		got := len(m.pushes)
		m.mu.Unlock()
		if got >= want {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.pushes))
	copy(out, m.pushes)
	return out
}

// newMonitorPushTestService 构造带在线主机与记录型 pusher 的集群服务。
// newMonitorPushTestService builds a cluster service with an online host and a recording pusher.
func newMonitorPushTestService(t *testing.T) (*Service, *Repository, *recordingMonitorPusher) {
	t.Helper()
	db, cleanup := setupServiceTestDB(t)
	t.Cleanup(cleanup)

	repo := NewRepository(db)
	hosts := NewMockHostProvider()
	now := time.Now()
	for id := uint(1); id <= 3; id++ {
		heartbeat := now
		hosts.AddHost(&HostInfo{
			ID:            id,
			Name:          fmt.Sprintf("host-%d", id),
			IPAddress:     fmt.Sprintf("10.0.0.%d", id),
			AgentID:       fmt.Sprintf("agent-%d", id),
			AgentStatus:   "installed",
			LastHeartbeat: &heartbeat,
		})
	}
	svc := NewService(repo, hosts, &ServiceConfig{HeartbeatTimeout: time.Hour})
	pusher := &recordingMonitorPusher{}
	svc.SetMonitorPusher(pusher)
	return svc, repo, pusher
}

func TestAddNodePushesMonitorConfig(t *testing.T) {
	svc, repo, pusher := newMonitorPushTestService(t)
	ctx := context.Background()

	cluster := &Cluster{Name: "mon-add", DeploymentMode: DeploymentModeHybrid, InstallDir: "/opt/seatunnel"}
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if _, err := svc.AddNode(ctx, cluster.ID, &AddNodeRequest{
		HostID:     2,
		Role:       NodeRoleMasterWorker,
		InstallDir: "/opt/seatunnel",
	}); err != nil {
		t.Fatalf("AddNode: %v", err)
	}

	pushes := pusher.waitFor(t, 1)
	want := fmt.Sprintf("%d/2", cluster.ID)
	if len(pushes) != 1 || pushes[0] != want {
		t.Fatalf("monitor pushes = %v, want [%s]", pushes, want)
	}
}

func TestAddNodesPushesMonitorConfigOnce(t *testing.T) {
	svc, repo, pusher := newMonitorPushTestService(t)
	ctx := context.Background()

	cluster := &Cluster{Name: "mon-add-batch", DeploymentMode: DeploymentModeSeparated, InstallDir: "/opt/seatunnel"}
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	nodes, err := svc.AddNodes(ctx, cluster.ID, &AddNodesRequest{
		HostID: 1,
		Entries: []AddNodeEntryRequest{
			{Role: NodeRoleMaster, InstallDir: "/opt/seatunnel"},
			{Role: NodeRoleWorker, InstallDir: "/opt/seatunnel"},
		},
	})
	if err != nil {
		t.Fatalf("AddNodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	pushes := pusher.waitFor(t, 1)
	want := fmt.Sprintf("%d/1", cluster.ID)
	if len(pushes) != 1 || pushes[0] != want {
		t.Fatalf("monitor pushes = %v, want single [%s]", pushes, want)
	}
}

func TestCreateClusterWithNodesPushesMonitorConfig(t *testing.T) {
	svc, _, pusher := newMonitorPushTestService(t)
	ctx := context.Background()

	cluster, err := svc.Create(ctx, &CreateClusterRequest{
		Name:           "mon-create",
		DeploymentMode: DeploymentModeHybrid,
		Nodes: []CreateNodeFromDiscovery{
			{HostID: 1, Role: "hybrid", InstallDir: "/opt/seatunnel"},
			{HostID: 2, Role: "hybrid", InstallDir: "/opt/seatunnel"},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	pushes := pusher.waitFor(t, 2)
	seen := map[string]bool{}
	for _, p := range pushes {
		seen[p] = true
	}
	for _, hostID := range []uint{1, 2} {
		want := fmt.Sprintf("%d/%d", cluster.ID, hostID)
		if !seen[want] {
			t.Fatalf("missing monitor push %s, got %v", want, pushes)
		}
	}
}

func TestUpdateNodePushesMonitorConfig(t *testing.T) {
	svc, repo, pusher := newMonitorPushTestService(t)
	ctx := context.Background()

	cluster := &Cluster{Name: "mon-update", DeploymentMode: DeploymentModeHybrid, InstallDir: "/opt/seatunnel"}
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	node := &ClusterNode{
		ClusterID:     cluster.ID,
		HostID:        1,
		Role:          NodeRoleMasterWorker,
		InstallDir:    "/opt/seatunnel",
		HazelcastPort: 5801,
		APIPort:       8080,
		WorkerPort:    5802,
	}
	if err := repo.AddNode(ctx, node); err != nil {
		t.Fatalf("add node: %v", err)
	}

	newDir := "/opt/seatunnel-3.1"
	if _, err := svc.UpdateNode(ctx, cluster.ID, node.ID, &UpdateNodeRequest{InstallDir: &newDir}); err != nil {
		t.Fatalf("UpdateNode: %v", err)
	}

	pushes := pusher.waitFor(t, 1)
	want := fmt.Sprintf("%d/1", cluster.ID)
	if len(pushes) != 1 || pushes[0] != want {
		t.Fatalf("monitor pushes = %v, want [%s]", pushes, want)
	}
}

func TestEnsureNodeForInstallationPushesMonitorConfig(t *testing.T) {
	svc, repo, pusher := newMonitorPushTestService(t)
	ctx := context.Background()

	cluster := &Cluster{Name: "mon-ensure", DeploymentMode: DeploymentModeHybrid, InstallDir: "/opt/seatunnel"}
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	// 新登记与刷新既有节点两条路径都要推送。
	// Both the register-new and refresh-existing paths must push.
	if err := svc.EnsureNodeForInstallation(ctx, cluster.ID, 3, "master/worker", "/opt/seatunnel", 5801, 8080, 5802, "/opt/jdk17"); err != nil {
		t.Fatalf("EnsureNodeForInstallation (new): %v", err)
	}
	if err := svc.EnsureNodeForInstallation(ctx, cluster.ID, 3, "master/worker", "/opt/seatunnel", 5801, 8080, 5802, "/opt/jdk21"); err != nil {
		t.Fatalf("EnsureNodeForInstallation (existing): %v", err)
	}

	pushes := pusher.waitFor(t, 2)
	want := fmt.Sprintf("%d/3", cluster.ID)
	if len(pushes) != 2 || pushes[0] != want || pushes[1] != want {
		t.Fatalf("monitor pushes = %v, want [%s %s]", pushes, want, want)
	}
}
