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

package main

import (
	"context"
	"testing"

	pb "github.com/LeonYoah/stx/agent"
	"github.com/LeonYoah/stx/agent/internal/config"
	"github.com/LeonYoah/stx/agent/internal/process"
	"github.com/LeonYoah/stx/internal/processidentity"
	"github.com/stretchr/testify/assert"
)

// TestHandleUpdateMonitorConfigEmptyListOnlyClearsTargetCluster 缩容后控制面按 cluster_id 下发空列表：
// 只移除该集群的跟踪项，同机其他集群的跟踪项（含 PID）保持不变。
// TestHandleUpdateMonitorConfigEmptyListOnlyClearsTargetCluster: after scale-in the Control Plane sends an
// empty list scoped by cluster_id; only that cluster's tracked entries are removed and other clusters on the
// same host (including their PIDs) are kept.
func TestHandleUpdateMonitorConfigEmptyListOnlyClearsTargetCluster(t *testing.T) {
	agent := NewAgent(&config.Config{})
	role := "master/worker"
	removedDir := "/tmp/seatunnel-3.0.0"
	keptDir := "/tmp/seatunnel-2.3.12"
	removedName := processidentity.ManagedName(removedDir, role)
	keptName := processidentity.ManagedName(keptDir, role)
	agent.processMonitor.TrackProcessSilent(removedName, 101, removedDir, role, &process.StartParams{})
	agent.processMonitor.SetProcessClusterID(removedName, "1")
	agent.processMonitor.TrackProcessSilent(keptName, 202, keptDir, role, &process.StartParams{})
	agent.processMonitor.SetProcessClusterID(keptName, "8")
	agent.matchSeaTunnelProcess = func(context.Context, int, string, string) (bool, error) { return true, nil }
	agent.findSeaTunnelProcess = func(context.Context, string, string) (int, string, error) {
		return 0, "", process.ErrProcessNotFound
	}

	response, err := agent.handleUpdateMonitorConfigCommand(context.Background(), &pb.CommandRequest{
		CommandId: "scale-in-cluster-1",
		Parameters: map[string]string{
			"cluster_id":        "1",
			"auto_monitor":      "true",
			"auto_restart":      "true",
			"tracked_processes": "[]",
		},
	}, noopProgressReporter{})

	assert.NoError(t, err)
	assert.Equal(t, pb.CommandStatus_SUCCESS, response.Status)
	assert.Nil(t, agent.processMonitor.GetTrackedProcess(removedName), "removed cluster's process must no longer be tracked")
	kept := agent.processMonitor.GetTrackedProcess(keptName)
	if assert.NotNil(t, kept, "other cluster's process must stay tracked") {
		assert.Equal(t, 202, kept.PID)
		assert.Equal(t, "8", kept.ClusterID)
	}
}
