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
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/gin-gonic/gin"
)

// 回归 #186：Agent 慢/超时时 handler 仍须立刻 2xx 受理，不能把已下发的重启报成 500。
// Regression #186: with a slow/timing-out Agent the handler must still 2xx-accept immediately
// and must not report a dispatched restart as HTTP 500.

func TestRestartNodeHandlerReturnsBeforeSlowAgent(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	hostProvider := NewMockHostProvider()
	now := time.Now()
	hostProvider.AddHost(&HostInfo{
		ID: 1, Name: "t4-dnode-1", IPAddress: "10.0.0.4",
		AgentID: "agent-1", LastHeartbeat: &now, HostType: "bare_metal",
	})

	var restartEntered sync.WaitGroup
	restartEntered.Add(1)
	var finished atomic.Bool
	sender := &scriptedAgentSender{
		send: func(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error) {
			if commandType == string(OperationRestart) {
				restartEntered.Done()
				time.Sleep(250 * time.Millisecond)
				finished.Store(true)
				return true, "ok", nil
			}
			if commandType == "check_process" {
				return true, "SeaTunnel process found: PID=4986, role=hybrid", nil
			}
			return true, "ok", nil
		},
	}

	svc := NewService(repo, hostProvider, &ServiceConfig{})
	svc.SetAgentCommandSender(sender)

	ctx := context.Background()
	cluster := &Cluster{
		Name: "t4-186-events", DeploymentMode: DeploymentModeHybrid,
		InstallDir: "/opt/seatunnel", Status: ClusterStatusRunning,
	}
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	node := &ClusterNode{
		ClusterID: cluster.ID, HostID: 1, Role: NodeRoleMasterWorker,
		InstallDir: "/opt/seatunnel", Status: NodeStatusRunning, ProcessPID: 4488,
	}
	if err := repo.AddNode(ctx, node); err != nil {
		t.Fatalf("create node: %v", err)
	}

	var hookCalls atomic.Int32
	handler := NewHandler(svc, nil)
	handler.SetOnOperationExecuted(func(context.Context, *OperationEvent) error {
		hookCalls.Add(1)
		return nil
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		auth.SetUserToContext(c, &auth.User{ID: 1, Username: "admin"})
		c.Next()
	})
	router.POST("/api/v1/clusters/:id/nodes/:nodeId/restart", handler.RestartNode)

	started := time.Now()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/clusters/"+strconv.FormatUint(uint64(cluster.ID), 10)+
			"/nodes/"+strconv.FormatUint(uint64(node.ID), 10)+"/restart",
		nil,
	)
	router.ServeHTTP(recorder, req)
	elapsed := time.Since(started)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if elapsed > 80*time.Millisecond {
		t.Fatalf("handler blocked too long: %s (want immediate accept)", elapsed)
	}
	if hookCalls.Load() != 1 {
		t.Fatalf("notify hook calls=%d, want 1", hookCalls.Load())
	}

	var body ClusterOperationResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data == nil || !body.Data.Success {
		t.Fatalf("accepted payload=%+v", body.Data)
	}

	updated, err := repo.GetNodeByID(ctx, node.ID)
	if err != nil {
		t.Fatalf("reload node: %v", err)
	}
	if updated.ProcessPID != 0 {
		t.Fatalf("pid cleared on accept? got %d", updated.ProcessPID)
	}

	select {
	case <-waitGroupDone(&restartEntered):
	case <-time.After(2 * time.Second):
		t.Fatal("background RestartNode never reached agent")
	}
	deadline := time.Now().Add(2 * time.Second)
	for !finished.Load() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !finished.Load() {
		t.Fatal("background agent restart did not finish")
	}
}

func TestRestartNodeTimeoutTreatedAsAccepted(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	hostProvider := NewMockHostProvider()
	now := time.Now()
	hostProvider.AddHost(&HostInfo{
		ID: 1, Name: "t4-dnode-1", IPAddress: "10.0.0.4",
		AgentID: "agent-1", LastHeartbeat: &now, HostType: "bare_metal",
	})

	sender := &scriptedAgentSender{
		send: func(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error) {
			if commandType == string(OperationRestart) {
				return false, "", errors.New("agent: command execution timeout")
			}
			if commandType == "check_process" {
				return true, "SeaTunnel process found: PID=4986, role=hybrid", nil
			}
			return true, "ok", nil
		},
	}

	svc := NewService(repo, hostProvider, &ServiceConfig{})
	svc.SetAgentCommandSender(sender)

	ctx := context.Background()
	cluster := &Cluster{
		Name: "t4-timeout", DeploymentMode: DeploymentModeHybrid,
		InstallDir: "/opt/seatunnel", Status: ClusterStatusRunning,
	}
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	node := &ClusterNode{
		ClusterID: cluster.ID, HostID: 1, Role: NodeRoleMasterWorker,
		InstallDir: "/opt/seatunnel", Status: NodeStatusRunning, ProcessPID: 4488,
	}
	if err := repo.AddNode(ctx, node); err != nil {
		t.Fatalf("create node: %v", err)
	}

	result, err := svc.RestartNode(ctx, cluster.ID, node.ID)
	if err != nil {
		t.Fatalf("RestartNode err=%v (timeout must be accepted)", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("result=%+v", result)
	}
	if !isAgentCommandTimeout(errors.New("agent: command execution timeout")) {
		t.Fatal("timeout detector broken")
	}
}

func TestClusterRestartTimeoutDoesNotMarkWholeOpFailed(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	hostProvider := NewMockHostProvider()
	now := time.Now()
	hostProvider.AddHost(&HostInfo{
		ID: 1, Name: "t4-dnode-1", IPAddress: "10.0.0.4",
		AgentID: "agent-1", LastHeartbeat: &now, HostType: "bare_metal",
	})

	sender := &scriptedAgentSender{
		send: func(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error) {
			if commandType == string(OperationRestart) {
				return false, "", errors.New("agent: command execution timeout")
			}
			if commandType == "check_process" {
				return true, "SeaTunnel process found: PID=4986, role=hybrid", nil
			}
			return true, "ok", nil
		},
	}

	svc := NewService(repo, hostProvider, &ServiceConfig{})
	svc.SetAgentCommandSender(sender)

	ctx := context.Background()
	cluster := &Cluster{
		Name: "t4-cluster-timeout", DeploymentMode: DeploymentModeHybrid,
		InstallDir: "/opt/seatunnel", Status: ClusterStatusRunning,
	}
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	node := &ClusterNode{
		ClusterID: cluster.ID, HostID: 1, Role: NodeRoleMasterWorker,
		InstallDir: "/opt/seatunnel", Status: NodeStatusRunning, ProcessPID: 4488,
	}
	if err := repo.AddNode(ctx, node); err != nil {
		t.Fatalf("create node: %v", err)
	}

	result, err := svc.Restart(ctx, cluster.ID)
	if err != nil {
		t.Fatalf("Restart err=%v", err)
	}
	if result == nil || !result.Success {
		t.Fatalf("want overall success on agent timeout, got %+v", result)
	}
	if result.Message == "Operation completed with errors" {
		t.Fatalf("timeout still surfaced as completed with errors: %+v", result)
	}
}

func waitGroupDone(wg *sync.WaitGroup) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		wg.Wait()
		close(ch)
	}()
	return ch
}
