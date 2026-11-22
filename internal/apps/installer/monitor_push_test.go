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
	"reflect"
	"sync"
	"testing"
	"time"
)

// recordingMonitorPusher 线程安全地记录监控配置推送（推送本身是异步的）。
// recordingMonitorPusher records monitor config pushes thread-safely (pushes run asynchronously).
type recordingMonitorPusher struct {
	mu     sync.Mutex
	pushes []string
	onPush func()
}

func (m *recordingMonitorPusher) PushClusterMonitorConfigToHost(_ context.Context, clusterID, hostID uint) error {
	m.mu.Lock()
	m.pushes = append(m.pushes, fmt.Sprintf("%d/%d", clusterID, hostID))
	cb := m.onPush
	m.mu.Unlock()
	if cb != nil {
		cb()
	}
	return nil
}

func (m *recordingMonitorPusher) waitFor(t *testing.T, want int) []string {
	t.Helper()
	if want > 0 {
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
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.pushes))
	copy(out, m.pushes)
	return out
}

// orderedInstallHarness 以互斥锁安全记录安装收尾步骤与异步监控推送的先后顺序。
// orderedInstallHarness mutex-protects the call sequence across synchronous install steps and async monitor push.
type orderedInstallHarness struct {
	mu       sync.Mutex
	calls    []string
	startOK  bool
	startMsg string
	startErr error
}

func (h *orderedInstallHarness) record(step string) {
	h.mu.Lock()
	h.calls = append(h.calls, step)
	h.mu.Unlock()
}

func (h *orderedInstallHarness) EnsureNodeForInstallation(context.Context, uint, uint, string, string, int, int, int, string) error {
	h.record("ensure_node")
	return nil
}

func (h *orderedInstallHarness) StartNodeByClusterAndHost(context.Context, uint, uint) (bool, string, error) {
	h.record("start_node")
	return h.startOK, h.startMsg, h.startErr
}

func (h *orderedInstallHarness) StartNodeByClusterAndHostAndRole(context.Context, uint, uint, string) (bool, string, error) {
	h.record("start_node")
	return h.startOK, h.startMsg, h.startErr
}

func (h *orderedInstallHarness) EnsureClusterConfigsForHost(context.Context, uint, uint, string, uint) error {
	h.record("ensure_configs")
	return nil
}

func (h *orderedInstallHarness) waitForCalls(t *testing.T, wantLen int) []string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		got := len(h.calls)
		h.mu.Unlock()
		if got >= wantLen {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.calls))
	copy(out, h.calls)
	return out
}

// 安装收尾必须在节点成功启动后才向该主机下发监控配置（直接启动路径），避免 AutoRestarter 与首次启动竞态。
// Post-install must push the monitor config only after node startup succeeds (direct-start path) so AutoRestarter never races with initial launch.
func TestStartClusterAfterInstallPushesMonitorConfig(t *testing.T) {
	harness := &orderedInstallHarness{startOK: true, startMsg: "started"}
	service := NewService(t.TempDir(), nil)
	service.SetNodeStarter(harness)
	service.SetConfigInitializer(harness)
	pusher := &recordingMonitorPusher{
		onPush: func() { harness.record("push_monitor") },
	}
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
	calls := harness.waitForCalls(t, 4)
	if want := []string{"ensure_node", "ensure_configs", "start_node", "push_monitor"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("post-install call order = %v, want %v", calls, want)
	}
	if status.Status != StepStatusSuccess {
		t.Fatalf("expected success status, got %s", status.Status)
	}
}

// 延迟启动路径（登记即返回、不启动节点）不得提前推送监控配置；由扩容编排器在启动后再推。
// The deferred-start path (register and return without starting) must not push monitor config before startup; scale executor pushes after starting.
func TestStartClusterAfterInstallSkipsMonitorPushOnDeferredStart(t *testing.T) {
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

	time.Sleep(50 * time.Millisecond)
	if pushes := pusher.waitFor(t, 0); len(pushes) != 0 {
		t.Fatalf("expected no monitor push on deferred start, got %v", pushes)
	}
	if status.Status != StepStatusSuccess {
		t.Fatalf("expected success status, got %s", status.Status)
	}
}

// 节点启动失败时不得推送监控配置，避免 Agent 把失败的半初始化节点纳入自动拉起。
// When node startup fails, monitor config must not be pushed so the Agent does not auto-restart a failed install.
func TestStartClusterAfterInstallSkipsMonitorPushWhenStartFails(t *testing.T) {
	harness := &orderedInstallHarness{startOK: false, startMsg: "start failed"}
	service := NewService(t.TempDir(), nil)
	service.SetNodeStarter(harness)
	service.SetConfigInitializer(harness)
	pusher := &recordingMonitorPusher{}
	service.SetMonitorPusher(pusher)
	status := &InstallationStatus{Status: StepStatusRunning}

	service.startClusterAfterInstall(context.Background(), "agent-test", &InstallationRequest{
		HostID:     "10",
		ClusterID:  "9",
		NodeRole:   NodeRoleMasterWorker,
		InstallDir: "/opt/seatunnel",
	}, status)

	time.Sleep(50 * time.Millisecond)
	if pushes := pusher.waitFor(t, 0); len(pushes) != 0 {
		t.Fatalf("expected no monitor push when startup fails, got %v", pushes)
	}
	if status.Status != StepStatusFailed {
		t.Fatalf("expected failed status, got %s", status.Status)
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
