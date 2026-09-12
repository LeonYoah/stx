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
	"testing"
	"time"
)

type stxJavaProxyAgentSender struct {
	lastAgentID        string
	lastCommand        string
	lastParams         map[string]string
	response           string
	responses          []string
	responsesByCommand map[string]string
	success            bool
	successes          []bool
	err                error
}

func (m *stxJavaProxyAgentSender) SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error) {
	m.lastAgentID = agentID
	m.lastCommand = commandType
	m.lastParams = map[string]string{}
	for k, v := range params {
		m.lastParams[k] = v
	}
	if len(m.responsesByCommand) > 0 {
		response, ok := m.responsesByCommand[commandType]
		if !ok {
			response = ""
		}
		success := m.success
		if len(m.successes) > 0 {
			success = m.successes[0]
			m.successes = m.successes[1:]
		}
		return success, response, m.err
	}
	if len(m.responses) > 0 {
		response := m.responses[0]
		m.responses = m.responses[1:]
		success := m.success
		if len(m.successes) > 0 {
			success = m.successes[0]
			m.successes = m.successes[1:]
		}
		return success, response, m.err
	}
	return m.success, m.response, m.err
}

func TestGetSTXJavaProxyStatusUsesOnlineMasterNode(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	hostProvider := NewMockHostProvider()
	service := NewService(repo, hostProvider, nil)
	lastHeartbeat := time.Now()
	hostProvider.AddHost(&HostInfo{
		ID:            1,
		Name:          "master-1",
		IPAddress:     "10.0.0.1",
		AgentID:       "agent-master-1",
		AgentStatus:   "installed",
		LastHeartbeat: &lastHeartbeat,
	})

	agentSender := &stxJavaProxyAgentSender{
		success:  true,
		response: `{"service":"stx_java_proxy","managed":true,"running":true,"healthy":true,"endpoint":"http://127.0.0.1:18080","port":18080,"pid":4567,"log_path":"/opt/seatunnel/.stx/stx-java-proxy/service.log","message":"ok"}`,
	}
	service.SetAgentCommandSender(agentSender)

	clusterInfo, err := service.Create(context.Background(), &CreateClusterRequest{
		Name:           "cluster-a",
		Version:        "2.3.13",
		InstallDir:     "/opt/seatunnel",
		DeploymentMode: DeploymentModeHybrid,
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if _, err := service.AddNode(context.Background(), clusterInfo.ID, &AddNodeRequest{
		HostID:     1,
		Role:       NodeRoleMasterWorker,
		InstallDir: "/opt/seatunnel",
	}); err != nil {
		t.Fatalf("add node: %v", err)
	}

	status, err := service.GetSTXJavaProxyStatus(context.Background(), clusterInfo.ID)
	if err != nil {
		t.Fatalf("GetSTXJavaProxyStatus returned error: %v", err)
	}
	if !status.Healthy || !status.Running {
		t.Fatalf("expected running healthy status, got %#v", status)
	}
	if agentSender.lastCommand != "status" {
		t.Fatalf("expected status command, got %s", agentSender.lastCommand)
	}
	if agentSender.lastParams["service"] != "stx_java_proxy" {
		t.Fatalf("expected service param stx_java_proxy, got %#v", agentSender.lastParams)
	}
}

func TestStartSTXJavaProxyPropagatesAgentFailure(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	hostProvider := NewMockHostProvider()
	service := NewService(repo, hostProvider, nil)
	lastHeartbeat := time.Now()
	hostProvider.AddHost(&HostInfo{
		ID:            1,
		Name:          "master-1",
		IPAddress:     "10.0.0.1",
		AgentID:       "agent-master-1",
		AgentStatus:   "installed",
		LastHeartbeat: &lastHeartbeat,
	})
	service.SetAgentCommandSender(&stxJavaProxyAgentSender{
		success:  false,
		response: `{"service":"stx_java_proxy","managed":true,"running":false,"healthy":false,"message":"start failed"}`,
	})

	clusterInfo, err := service.Create(context.Background(), &CreateClusterRequest{
		Name:           "cluster-a",
		Version:        "2.3.13",
		InstallDir:     "/opt/seatunnel",
		DeploymentMode: DeploymentModeHybrid,
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if _, err := service.AddNode(context.Background(), clusterInfo.ID, &AddNodeRequest{
		HostID:     1,
		Role:       NodeRoleMasterWorker,
		InstallDir: "/opt/seatunnel",
	}); err != nil {
		t.Fatalf("add node: %v", err)
	}

	status, err := service.StartSTXJavaProxy(context.Background(), clusterInfo.ID)
	if err == nil {
		t.Fatal("expected error from failed start command")
	}
	if status == nil || status.Message != "start failed" {
		t.Fatalf("expected decoded failure status, got %#v", status)
	}
}

func TestGetSTXJavaProxyServiceLogUsesReportedLogPath(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	hostProvider := NewMockHostProvider()
	service := NewService(repo, hostProvider, nil)
	lastHeartbeat := time.Now()
	hostProvider.AddHost(&HostInfo{
		ID:            1,
		Name:          "master-1",
		IPAddress:     "10.0.0.1",
		AgentID:       "agent-master-1",
		AgentStatus:   "installed",
		LastHeartbeat: &lastHeartbeat,
	})

	agentSender := &stxJavaProxyAgentSender{
		success: true,
		responsesByCommand: map[string]string{
			"status":   `{"service":"stx_java_proxy","managed":true,"running":true,"healthy":true,"endpoint":"http://127.0.0.1:18080","port":18080,"pid":4567,"log_path":"/opt/seatunnel/.stx/stx-java-proxy/service.log","message":"ok"}`,
			"get_logs": "line-1\nline-2",
		},
	}
	service.SetAgentCommandSender(agentSender)

	clusterInfo, err := service.Create(context.Background(), &CreateClusterRequest{
		Name:           "cluster-a",
		Version:        "2.3.13",
		InstallDir:     "/opt/seatunnel",
		DeploymentMode: DeploymentModeHybrid,
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if _, err := service.AddNode(context.Background(), clusterInfo.ID, &AddNodeRequest{
		HostID:     1,
		Role:       NodeRoleMasterWorker,
		InstallDir: "/opt/seatunnel",
	}); err != nil {
		t.Fatalf("add node: %v", err)
	}

	result, err := service.GetSTXJavaProxyServiceLog(context.Background(), clusterInfo.ID, 120)
	if err != nil {
		t.Fatalf("GetSTXJavaProxyServiceLog returned error: %v", err)
	}
	if result.LogPath != "/opt/seatunnel/.stx/stx-java-proxy/service.log" {
		t.Fatalf("unexpected log path: %#v", result)
	}
	if result.Logs != "line-1\nline-2" {
		t.Fatalf("unexpected log payload: %#v", result)
	}
	if agentSender.lastCommand != "get_logs" {
		t.Fatalf("expected get_logs command, got %s", agentSender.lastCommand)
	}
	if agentSender.lastParams["log_file"] != result.LogPath {
		t.Fatalf("expected get_logs to target reported log path, got %#v", agentSender.lastParams)
	}
}
