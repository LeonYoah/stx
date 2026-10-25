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
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// orderRecorder 记录安装后各步骤的调用顺序。
// orderRecorder records the call order of post-install steps.
type orderRecorder struct {
	calls []string
}

type orderedNodeStarter struct {
	recorder *orderRecorder
}

func (s *orderedNodeStarter) EnsureNodeForInstallation(context.Context, uint, uint, string, string, int, int, int) error {
	s.recorder.calls = append(s.recorder.calls, "ensure_node")
	return nil
}

func (s *orderedNodeStarter) StartNodeByClusterAndHost(context.Context, uint, uint) (bool, string, error) {
	s.recorder.calls = append(s.recorder.calls, "start_node")
	return true, "started", nil
}

func (s *orderedNodeStarter) StartNodeByClusterAndHostAndRole(context.Context, uint, uint, string) (bool, string, error) {
	s.recorder.calls = append(s.recorder.calls, "start_node")
	return true, "started", nil
}

type orderedConfigInitializer struct {
	recorder *orderRecorder
}

func (c *orderedConfigInitializer) EnsureClusterConfigsForHost(context.Context, uint, uint, string, uint) error {
	c.recorder.calls = append(c.recorder.calls, "ensure_configs")
	return nil
}

func newOrderedInstallerService(t *testing.T) (*Service, *orderRecorder) {
	t.Helper()
	recorder := &orderRecorder{}
	service := NewService(t.TempDir(), nil)
	service.SetNodeStarter(&orderedNodeStarter{recorder: recorder})
	service.SetConfigInitializer(&orderedConfigInitializer{recorder: recorder})
	return service, recorder
}

func TestStartClusterAfterInstallRegistersBeforeStartAndEnsuresConfigs(t *testing.T) {
	service, recorder := newOrderedInstallerService(t)
	status := &InstallationStatus{Status: StepStatusRunning}

	service.startClusterAfterInstall(context.Background(), "agent-test", &InstallationRequest{
		HostID:     "10",
		ClusterID:  "9",
		NodeRole:   NodeRoleMasterWorker,
		InstallDir: "/opt/seatunnel",
	}, status)

	if want := []string{"ensure_node", "start_node", "ensure_configs"}; !reflect.DeepEqual(recorder.calls, want) {
		t.Fatalf("post-install call order = %v, want %v", recorder.calls, want)
	}
	if status.Status != StepStatusSuccess || status.CurrentStep != InstallStepComplete {
		t.Fatalf("expected success/complete, got %s/%s", status.Status, status.CurrentStep)
	}
}

func TestStartClusterAfterInstallDeferNodeStartSkipsStartAndConfigs(t *testing.T) {
	service, recorder := newOrderedInstallerService(t)
	status := &InstallationStatus{Status: StepStatusRunning}

	service.startClusterAfterInstall(context.Background(), "agent-test", &InstallationRequest{
		HostID:         "10",
		ClusterID:      "9",
		NodeRole:       NodeRoleMasterWorker,
		InstallDir:     "/opt/seatunnel",
		DeferNodeStart: true,
	}, status)

	if want := []string{"ensure_node"}; !reflect.DeepEqual(recorder.calls, want) {
		t.Fatalf("deferred install calls = %v, want %v (no start, no config registration)", recorder.calls, want)
	}
	if status.Status != StepStatusSuccess {
		t.Fatalf("expected success status, got %s", status.Status)
	}
	if status.CurrentStep != InstallStepComplete {
		t.Fatalf("expected current step %q, got %q", InstallStepComplete, status.CurrentStep)
	}
	if status.Error != "" || status.EndTime == nil {
		t.Fatalf("expected no error and an end time, got error=%q end=%v", status.Error, status.EndTime)
	}
	if !strings.Contains(status.Message, "deferred") {
		t.Fatalf("expected deferred-start message, got %q", status.Message)
	}
}

func TestStartClusterAfterInstallDeferNodeStartStillFailsOnRegistrationError(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	starter := &stubInstallationNodeStarter{ensureErr: context.DeadlineExceeded}
	service.SetNodeStarter(starter)
	status := &InstallationStatus{Status: StepStatusRunning}

	service.startClusterAfterInstall(context.Background(), "agent-test", &InstallationRequest{
		HostID:         "10",
		ClusterID:      "9",
		NodeRole:       NodeRoleMasterWorker,
		DeferNodeStart: true,
	}, status)

	if status.Status != StepStatusFailed {
		t.Fatalf("expected failed status when registration fails, got %s", status.Status)
	}
	if starter.startCalls != 0 {
		t.Fatalf("expected no start call, got %d", starter.startCalls)
	}
}

func TestDeferNodeStartIsNotPartOfPublicJSON(t *testing.T) {
	var req InstallationRequest
	if err := json.Unmarshal([]byte(`{"version":"2.3.13","defer_node_start":true,"DeferNodeStart":true}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.DeferNodeStart {
		t.Fatal("DeferNodeStart must not be settable through public JSON")
	}
	out, err := json.Marshal(&InstallationRequest{Version: "2.3.13", DeferNodeStart: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(strings.ToLower(string(out)), "defer") {
		t.Fatalf("DeferNodeStart leaked into JSON: %s", out)
	}
}

func TestGetInstallationStatusSnapshotIsDeepCopy(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	if _, err := service.GetInstallationStatusSnapshot(context.Background(), 7); err != ErrInstallationNotFound {
		t.Fatalf("expected ErrInstallationNotFound, got %v", err)
	}
	live := &InstallationStatus{
		ID:       "install-1",
		HostID:   "7",
		Status:   StepStatusRunning,
		Steps:    []StepInfo{{Step: InstallStepDownload, Status: StepStatusRunning}},
		Warnings: []string{"slow disk"},
	}
	service.installMu.Lock()
	service.installations["7"] = live
	service.installMu.Unlock()

	snapshot, err := service.GetInstallationStatusSnapshot(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetInstallationStatusSnapshot: %v", err)
	}
	if snapshot == live || snapshot.ID != "install-1" || snapshot.Status != StepStatusRunning {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	// 修改快照不影响实时状态。
	// Mutating the snapshot never touches the live status.
	snapshot.Steps[0].Status = StepStatusFailed
	snapshot.Warnings[0] = "changed"
	if live.Steps[0].Status != StepStatusRunning || live.Warnings[0] != "slow disk" {
		t.Fatal("snapshot must not share slices with the live status")
	}
}
