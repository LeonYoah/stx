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

package installer

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

// 安装收尾在节点登记后立刻向该主机下发监控配置（直接启动路径）。
// Post-install pushes the monitor config to the host right after node registration (direct-start path).
func TestStartClusterAfterInstallPushesMonitorConfig(t *testing.T) {
	service, _ := newOrderedInstallerService(t)
	pusher := &recordingMonitorPusher{}
	service.SetMonitorPusher(pusher)
	status := &InstallationStatus{Status: StepStatusRunning}

	service.startClusterAfterInstall(context.Background(), "agent-test", &InstallationRequest{
		HostID:     "10",
		ClusterID:  "9",
		NodeRole:   NodeRoleMasterWorker,
		InstallDir: "/opt/seatunnel",
	}, status)

	pushes := pusher.waitFor(t, 1)
	if len(pushes) != 1 || pushes[0] != "9/10" {
		t.Fatalf("monitor pushes = %v, want [9/10]", pushes)
	}
	if status.Status != StepStatusSuccess {
		t.Fatalf("expected success status, got %s", status.Status)
	}
}

// 延迟启动路径（登记即返回、不启动节点）同样要推送监控配置。
// The deferred-start path (register and return without starting) must also push the monitor config.
func TestStartClusterAfterInstallPushesMonitorConfigOnDeferredStart(t *testing.T) {
	service, _ := newOrderedInstallerService(t)
	pusher := &recordingMonitorPusher{}
	service.SetMonitorPusher(pusher)
	status := &InstallationStatus{Status: StepStatusRunning}

	service.startClusterAfterInstall(context.Background(), "agent-test", &InstallationRequest{
		HostID:         "10",
		ClusterID:      "9",
		NodeRole:       NodeRoleMasterWorker,
		InstallDir:     "/opt/seatunnel",
		DeferNodeStart: true,
	}, status)

	pushes := pusher.waitFor(t, 1)
	if len(pushes) != 1 || pushes[0] != "9/10" {
		t.Fatalf("monitor pushes = %v, want [9/10]", pushes)
	}
	if status.Status != StepStatusSuccess {
		t.Fatalf("expected success status, got %s", status.Status)
	}
}

// 未注入 pusher 时安装流程不受影响。
// Install flow is unaffected when no pusher is injected.
func TestStartClusterAfterInstallWithoutMonitorPusher(t *testing.T) {
	service, _ := newOrderedInstallerService(t)
	status := &InstallationStatus{Status: StepStatusRunning}

	service.startClusterAfterInstall(context.Background(), "agent-test", &InstallationRequest{
		HostID:     "10",
		ClusterID:  "9",
		NodeRole:   NodeRoleMasterWorker,
		InstallDir: "/opt/seatunnel",
	}, status)

	if status.Status != StepStatusSuccess {
		t.Fatalf("expected success status, got %s", status.Status)
	}
}
