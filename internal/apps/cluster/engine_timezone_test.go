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

func TestEngineLocationUsesOnlineMasterHeartbeatTimezone(t *testing.T) {
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
		Timezone:      "Asia/Shanghai",
	})
	hostProvider.AddHost(&HostInfo{
		ID:            2,
		Name:          "worker-1",
		IPAddress:     "10.0.0.2",
		AgentID:       "agent-worker-1",
		AgentStatus:   "installed",
		LastHeartbeat: &lastHeartbeat,
		Timezone:      "UTC",
	})

	clusterInfo, err := service.Create(context.Background(), &CreateClusterRequest{
		Name:           "tz-cluster",
		Version:        "2.3.13",
		InstallDir:     "/opt/seatunnel",
		DeploymentMode: DeploymentModeSeparated,
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if _, err := service.AddNode(context.Background(), clusterInfo.ID, &AddNodeRequest{
		HostID:     1,
		Role:       NodeRoleMaster,
		InstallDir: "/opt/seatunnel",
	}); err != nil {
		t.Fatalf("add master: %v", err)
	}
	if _, err := service.AddNode(context.Background(), clusterInfo.ID, &AddNodeRequest{
		HostID:     2,
		Role:       NodeRoleWorker,
		InstallDir: "/opt/seatunnel",
	}); err != nil {
		t.Fatalf("add worker: %v", err)
	}

	loc := service.EngineLocation(context.Background(), clusterInfo.ID)
	if loc == nil || loc.String() != "Asia/Shanghai" {
		t.Fatalf("expected Asia/Shanghai from master heartbeat, got %v", loc)
	}
}
