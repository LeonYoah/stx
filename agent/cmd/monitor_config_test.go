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
	"encoding/json"
	"testing"

	pb "github.com/LeonYoah/stx/agent"
	"github.com/LeonYoah/stx/agent/internal/config"
	"github.com/LeonYoah/stx/agent/internal/process"
	"github.com/LeonYoah/stx/internal/processidentity"
	"github.com/stretchr/testify/assert"
)

func TestResolveMonitorProcessPIDPrefersVerifiedLocalPID(t *testing.T) {
	agent := NewAgent(&config.Config{})
	installDir := "/tmp/seatunnel-2.3.12"
	role := "master/worker"
	processName := processidentity.ManagedName(installDir, role)
	agent.processMonitor.TrackProcessSilent(processName, 101, installDir, role, &process.StartParams{})
	agent.matchSeaTunnelProcess = func(_ context.Context, pid int, _, _ string) (bool, error) {
		return pid == 101, nil
	}
	agent.findSeaTunnelProcess = func(context.Context, string, string) (int, string, error) {
		t.Fatal("本地 PID 已匹配时不应再次扫描进程 / process discovery should not run when the local PID matches")
		return 0, "", nil
	}

	resolvedPID := agent.resolveMonitorProcessPID(context.Background(), processName, 202, installDir, role)

	assert.Equal(t, 101, resolvedPID)
}

func TestResolveMonitorProcessPIDRejectsStaleConfiguredPIDAndDiscoversCurrentProcess(t *testing.T) {
	agent := NewAgent(&config.Config{})
	installDir := "/tmp/seatunnel-2.3.12"
	role := "master/worker"
	processName := processidentity.ManagedName(installDir, role)
	agent.matchSeaTunnelProcess = func(_ context.Context, pid int, _, _ string) (bool, error) {
		return false, nil
	}
	agent.findSeaTunnelProcess = func(_ context.Context, gotInstallDir, gotRole string) (int, string, error) {
		assert.Equal(t, installDir, gotInstallDir)
		assert.Equal(t, role, gotRole)
		return 303, "java ...", nil
	}

	resolvedPID := agent.resolveMonitorProcessPID(context.Background(), processName, 202, installDir, role)

	assert.Equal(t, 303, resolvedPID)
}

func TestResolveMonitorProcessPIDReturnsZeroWhenNoMatchingProcessExists(t *testing.T) {
	agent := NewAgent(&config.Config{})
	installDir := "/tmp/seatunnel-2.3.12"
	role := "master/worker"
	processName := processidentity.ManagedName(installDir, role)
	agent.matchSeaTunnelProcess = func(_ context.Context, pid int, _, _ string) (bool, error) {
		return false, nil
	}
	agent.findSeaTunnelProcess = func(context.Context, string, string) (int, string, error) {
		return 0, "", process.ErrProcessNotFound
	}

	resolvedPID := agent.resolveMonitorProcessPID(context.Background(), processName, 202, installDir, role)

	assert.Zero(t, resolvedPID)
}

func TestHandleUpdateMonitorConfigKeepsOtherClustersTracked(t *testing.T) {
	agent := NewAgent(&config.Config{})
	cluster6Dir := "/tmp/seatunnel-2.3.13"
	cluster8Dir := "/tmp/seatunnel-2.3.12"
	role := "master/worker"
	cluster6Name := processidentity.ManagedName(cluster6Dir, role)
	cluster8Name := processidentity.ManagedName(cluster8Dir, role)
	agent.processMonitor.TrackProcessSilent(cluster6Name, 606, cluster6Dir, role, &process.StartParams{})
	agent.processMonitor.SetProcessClusterID(cluster6Name, "6")
	agent.processMonitor.TrackProcessSilent(cluster8Name, 808, cluster8Dir, role, &process.StartParams{})
	agent.processMonitor.SetProcessClusterID(cluster8Name, "8")
	agent.matchSeaTunnelProcess = func(_ context.Context, pid int, _, _ string) (bool, error) {
		return pid == 606 || pid == 808, nil
	}
	agent.findSeaTunnelProcess = func(context.Context, string, string) (int, string, error) {
		return 0, "", process.ErrProcessNotFound
	}

	processesJSON, err := json.Marshal([]map[string]interface{}{
		{
			"pid":         999,
			"name":        cluster8Name,
			"cluster_id":  8,
			"install_dir": cluster8Dir,
			"role":        role,
		},
	})
	assert.NoError(t, err)

	response, err := agent.handleUpdateMonitorConfigCommand(context.Background(), &pb.CommandRequest{
		CommandId: "monitor-config-cluster-8",
		Parameters: map[string]string{
			"cluster_id":        "8",
			"auto_monitor":      "true",
			"auto_restart":      "true",
			"tracked_processes": string(processesJSON),
		},
	}, noopProgressReporter{})

	assert.NoError(t, err)
	assert.Equal(t, pb.CommandStatus_SUCCESS, response.Status)
	assert.Equal(t, 606, agent.processMonitor.GetTrackedProcess(cluster6Name).PID)
	assert.Equal(t, "6", agent.processMonitor.GetTrackedProcess(cluster6Name).ClusterID)
	assert.Equal(t, 808, agent.processMonitor.GetTrackedProcess(cluster8Name).PID)
	assert.Equal(t, "8", agent.processMonitor.GetTrackedProcess(cluster8Name).ClusterID)
}
