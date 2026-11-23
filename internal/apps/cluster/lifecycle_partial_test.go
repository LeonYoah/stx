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
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestIsIdempotentLifecycleOutcome(t *testing.T) {
	t.Parallel()
	if !isIdempotentLifecycleOutcome(OperationStart, "process is already running") {
		t.Fatal("start already-running must be idempotent")
	}
	if isIdempotentLifecycleOutcome(OperationStart, "agent not connected") {
		t.Fatal("agent disconnect must not be idempotent")
	}
	if !isIdempotentLifecycleOutcome(OperationStop, "process not found") {
		t.Fatal("stop not-found must be idempotent")
	}
}

func TestFinalizeLifecycleOperationResultPartial(t *testing.T) {
	t.Parallel()
	result := &OperationResult{
		NodeResults: []*NodeOperationResult{
			{NodeID: 1, Success: true},
			{NodeID: 2, Success: false, Message: "agent not connected"},
			{NodeID: 3, Success: true},
		},
	}
	finalizeLifecycleOperationResult(result)
	if result.Success {
		t.Fatal("partial failure must not set overall Success=true")
	}
	if !strings.Contains(result.Message, "2/3") {
		t.Fatalf("expected 2/3 in message, got %q", result.Message)
	}
}

func TestClusterStartTreatsAlreadyRunningAsSuccessAndKeepsPartialFailure(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	hosts := NewMockHostProvider()
	now := time.Now()
	for id, name := range map[uint]string{1: "n1", 2: "n2", 3: "n3"} {
		hosts.AddHost(&HostInfo{
			ID: id, Name: name, HostType: "bare_metal", IPAddress: fmt.Sprintf("10.0.0.%d", id),
			AgentID: "agent-" + name, AgentStatus: "installed", LastHeartbeat: &now,
		})
	}

	svc := NewService(repo, hosts, nil)
	sender := &scriptedAgentSender{send: func(_ context.Context, agentID, commandType string, _ map[string]string) (bool, string, error) {
		if commandType != string(OperationStart) {
			return true, "ok", nil
		}
		switch agentID {
		case "agent-n1", "agent-n3":
			return false, "process is already running", nil
		case "agent-n2":
			return false, "", errors.New("agent: agent not connected")
		default:
			return true, "ok", nil
		}
	}}
	svc.SetAgentCommandSender(sender)

	ctx := context.Background()
	cluster := &Cluster{
		Name: "partial-start", DeploymentMode: DeploymentModeHybrid,
		InstallDir: "/opt/seatunnel", Status: ClusterStatusRunning,
	}
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatal(err)
	}
	for hostID, status := range map[uint]NodeStatus{1: NodeStatusRunning, 2: NodeStatusStopped, 3: NodeStatusRunning} {
		node := &ClusterNode{
			ClusterID: cluster.ID, HostID: hostID, Role: NodeRoleMasterWorker,
			InstallDir: "/opt/seatunnel", HazelcastPort: 5801, APIPort: 8080,
			Status: status, ProcessPID: 100 + int(hostID),
		}
		if err := repo.AddNode(ctx, node); err != nil {
			t.Fatal(err)
		}
	}

	result, err := svc.Start(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if result.Success {
		t.Fatal("overall start must be partial failure")
	}
	if !strings.Contains(result.Message, "2/3") {
		t.Fatalf("message=%q", result.Message)
	}

	nodes, err := repo.GetNodesByClusterID(ctx, cluster.ID)
	if err != nil {
		t.Fatal(err)
	}
	byHost := map[uint]NodeStatus{}
	for _, n := range nodes {
		byHost[n.HostID] = n.Status
	}
	if byHost[1] != NodeStatusRunning || byHost[3] != NodeStatusRunning {
		t.Fatalf("already-running nodes must stay running, got %+v", byHost)
	}
	if byHost[2] != NodeStatusError {
		t.Fatalf("disconnected node must be error, got %s", byHost[2])
	}
}

func TestClusterStopSuccessLeavesStoppedNotError(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	hosts := NewMockHostProvider()
	now := time.Now()
	hosts.AddHost(&HostInfo{
		ID: 1, Name: "n1", HostType: "bare_metal", IPAddress: "10.0.0.1",
		AgentID: "agent-n1", AgentStatus: "installed", LastHeartbeat: &now,
	})
	hosts.AddHost(&HostInfo{
		ID: 2, Name: "n2", HostType: "bare_metal", IPAddress: "10.0.0.2",
		AgentID: "agent-n2", AgentStatus: "installed", LastHeartbeat: &now,
	})

	svc := NewService(repo, hosts, nil)
	svc.SetAgentCommandSender(&scriptedAgentSender{send: func(_ context.Context, _, commandType string, _ map[string]string) (bool, string, error) {
		if commandType == string(OperationStop) {
			return true, "Process stopped successfully", nil
		}
		return true, "ok", nil
	}})

	ctx := context.Background()
	cluster := &Cluster{
		Name: "stop-ok", DeploymentMode: DeploymentModeHybrid,
		InstallDir: "/opt/seatunnel", Status: ClusterStatusRunning,
	}
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatal(err)
	}
	for _, hostID := range []uint{1, 2} {
		if err := repo.AddNode(ctx, &ClusterNode{
			ClusterID: cluster.ID, HostID: hostID, Role: NodeRoleMasterWorker,
			InstallDir: "/opt/seatunnel", HazelcastPort: 5801, Status: NodeStatusRunning, ProcessPID: 50,
		}); err != nil {
			t.Fatal(err)
		}
	}

	result, err := svc.Stop(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !result.Success {
		t.Fatalf("stop should succeed: %+v", result)
	}

	updated, err := repo.GetByID(ctx, cluster.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != ClusterStatusStopped {
		t.Fatalf("cluster status=%s, want stopped", updated.Status)
	}
	for _, n := range updated.Nodes {
		if n.Status != NodeStatusStopped {
			t.Fatalf("node %d status=%s, want stopped (not error)", n.ID, n.Status)
		}
	}
}

func TestClusterStopPartialFailureOnlyMarksFailedNodeError(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	hosts := NewMockHostProvider()
	now := time.Now()
	hosts.AddHost(&HostInfo{
		ID: 1, Name: "n1", HostType: "bare_metal", IPAddress: "10.0.0.1",
		AgentID: "agent-n1", AgentStatus: "installed", LastHeartbeat: &now,
	})
	hosts.AddHost(&HostInfo{
		ID: 2, Name: "n2", HostType: "bare_metal", IPAddress: "10.0.0.2",
		AgentID: "agent-n2", AgentStatus: "installed", LastHeartbeat: &now,
	})

	svc := NewService(repo, hosts, nil)
	svc.SetAgentCommandSender(&scriptedAgentSender{send: func(_ context.Context, agentID, commandType string, _ map[string]string) (bool, string, error) {
		if commandType == string(OperationStop) && agentID == "agent-n2" {
			return false, "stop failed: permission denied", nil
		}
		return true, "ok", nil
	}})

	ctx := context.Background()
	cluster := &Cluster{
		Name: "stop-partial", DeploymentMode: DeploymentModeHybrid,
		InstallDir: "/opt/seatunnel", Status: ClusterStatusRunning,
	}
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatal(err)
	}
	for _, hostID := range []uint{1, 2} {
		if err := repo.AddNode(ctx, &ClusterNode{
			ClusterID: cluster.ID, HostID: hostID, Role: NodeRoleMasterWorker,
			InstallDir: "/opt/seatunnel", HazelcastPort: 5801, Status: NodeStatusRunning,
		}); err != nil {
			t.Fatal(err)
		}
	}

	result, err := svc.Stop(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if result.Success || !strings.Contains(result.Message, "1/2") {
		t.Fatalf("expected partial 1/2 failure, got %+v", result)
	}

	nodes, err := repo.GetNodesByClusterID(ctx, cluster.ID)
	if err != nil {
		t.Fatal(err)
	}
	byHost := map[uint]NodeStatus{}
	for _, n := range nodes {
		byHost[n.HostID] = n.Status
	}
	if byHost[1] != NodeStatusStopped {
		t.Fatalf("successful stop node must be stopped, got %s", byHost[1])
	}
	if byHost[2] != NodeStatusError {
		t.Fatalf("failed stop node must be error, got %s", byHost[2])
	}
}
