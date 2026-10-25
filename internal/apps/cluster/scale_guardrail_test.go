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
	"strings"
	"testing"

	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	"github.com/gin-gonic/gin"
)

// TestScaleInFingerprintTracksJobPresenceNotIDs 缩容指纹只随「有无运行中作业」变化，不随作业 ID 变化。
// TestScaleInFingerprintTracksJobPresenceNotIDs checks the scale-in fingerprint follows "any running jobs", not job IDs.
func TestScaleInFingerprintTracksJobPresenceNotIDs(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker, 2: NodeRoleMasterWorker})
	node, _ := env.svc.repo.GetNodeByClusterAndHostAndRole(ctx, env.cluster.ID, 2, string(NodeRoleMasterWorker))
	plan := func() *ScalePlan {
		t.Helper()
		result, err := env.scale.PlanScaleIn(ctx, env.cluster.ID, node.ID, "en")
		if err != nil {
			t.Fatalf("PlanScaleIn: %v", err)
		}
		return result
	}

	env.jobs.jobs = []ScaleRunningJob{{JobID: "1", JobName: "batch-a"}}
	first := plan()
	env.jobs.jobs = []ScaleRunningJob{{JobID: "2", JobName: "batch-b"}, {JobID: "3", JobName: "batch-c"}}
	replaced := plan()
	if first.Fingerprint != replaced.Fingerprint {
		t.Fatal("different job IDs with jobs still present must keep the fingerprint")
	}
	env.jobs.jobs = nil
	none := plan()
	if none.Fingerprint == first.Fingerprint {
		t.Fatal("going from running jobs to none must change the fingerprint")
	}
	env.jobs.jobs = []ScaleRunningJob{{JobID: "9"}}
	if plan().Fingerprint == none.Fingerprint {
		t.Fatal("going from none to running jobs must change the fingerprint")
	}
}

// TestScaleOutLoopbackFingerprintIgnoresJobIDs 扩容存在回环节点时同样只记录作业有无。
// TestScaleOutLoopbackFingerprintIgnoresJobIDs applies the same rule to scale-out with loopback nodes.
func TestScaleOutLoopbackFingerprintIgnoresJobIDs(t *testing.T) {
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	env.agent.setMembers([]string{"127.0.0.1:5801"}, "agent-1")
	env.jobs.jobs = []ScaleRunningJob{{JobID: "1"}}
	first := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	env.jobs.jobs = []ScaleRunningJob{{JobID: "2"}}
	second := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	if len(first.LoopbackNodes) == 0 {
		t.Fatal("expected loopback nodes")
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatal("job IDs must not affect the scale-out fingerprint")
	}
}

// TestPlanScaleOutLoopbackWarnsWhenJobsUnknown 回环修正 + 作业未知：不阻断、仍需重启确认、给出醒目警告。
// TestPlanScaleOutLoopbackWarnsWhenJobsUnknown: loopback fix with unknown jobs is not blocked, still needs the restart
// acknowledgement and shows a prominent warning.
func TestPlanScaleOutLoopbackWarnsWhenJobsUnknown(t *testing.T) {
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	hostID := uint(1)
	env.config.configs = append(env.config.configs, &appconfig.ConfigInfo{ID: 10, ClusterID: env.cluster.ID, HostID: &hostID, ConfigType: appconfig.ConfigTypeHazelcast, Content: hazelcastMembers("127.0.0.1:5801")})
	env.jobs.err = errors.New("engine http disabled")

	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	if plan.RunningJobsKnown || !plan.RequiresRestartAck || !plan.CanSubmit {
		t.Fatalf("unknown jobs with loopback: known=%v ack=%v can=%v", plan.RunningJobsKnown, plan.RequiresRestartAck, plan.CanSubmit)
	}
	checks := findCheck(plan, "loopback_restart_jobs_unknown")
	if len(checks) != 1 || checks[0].Status != ScaleCheckWarning || checks[0].Blocking {
		t.Fatalf("loopback_restart_jobs_unknown = %+v", checks)
	}
	found := false
	for _, warning := range plan.Warnings {
		if warning == checks[0].Message {
			found = true
		}
	}
	if !found {
		t.Fatalf("the warning must be listed in plan.warnings: %v", plan.Warnings)
	}

	// 作业已知时不出现该警告。
	// The warning is absent when jobs are known.
	env.jobs.err = nil
	known := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	if len(findCheck(known, "loopback_restart_jobs_unknown")) != 0 {
		t.Fatal("no unknown-jobs warning when jobs are known")
	}
}

// TestPlanScaleOutRegisterWarnsWhenTargetNotJoined 纳管节点自成集群时给出不阻断的组网警告。
// TestPlanScaleOutRegisterWarnsWhenTargetNotJoined warns (non-blocking) when the registered node forms its own cluster.
func TestPlanScaleOutRegisterWarnsWhenTargetNotJoined(t *testing.T) {
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	env.discover.processes[2] = []ScaleDiscoveredProcess{{PID: 9, Role: "hybrid", InstallDir: "/data/seatunnel", Version: env.cluster.Version, HazelcastPort: 5901, APIPort: 8090}}
	env.agent.listening["agent-2|5901"] = true

	env.agent.setMembers([]string{"10.0.0.2:5901"}, "agent-2")
	alone := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	checks := findCheck(alone, "target_joined_cluster")
	if len(checks) != 1 || checks[0].Status != ScaleCheckWarning || checks[0].Blocking || !alone.CanSubmit {
		t.Fatalf("separate cluster must warn without blocking: %+v can=%v", checks, alone.CanSubmit)
	}

	env.agent.setMembers([]string{"10.0.0.1:5801", "10.0.0.2:5901"}, "agent-2")
	joined := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	if checks := findCheck(joined, "target_joined_cluster"); len(checks) != 1 || checks[0].Status != ScaleCheckPassed {
		t.Fatalf("joined node must pass: %+v", checks)
	}

	// 安装模式不做该检查。
	// Install mode does not run the check.
	install := env.mustPlanOut(t, ScaleOutRequest{HostID: 2, Mode: ScaleModeInstall, InstallDir: "/opt/other", Ports: ScaleNodePorts{HazelcastPort: 5911}})
	if len(findCheck(install, "target_joined_cluster")) != 0 {
		t.Fatal("install mode must not check target membership")
	}
}

// TestScaleInProgressErrorCarriesInitiator 409 冲突携带进行中操作的编号、类型与发起人。
// TestScaleInProgressErrorCarriesInitiator checks the 409 conflict carries the running operation's ID, kind and initiator.
func TestScaleInProgressErrorCarriesInitiator(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	running := &ClusterScaleOperation{ClusterID: env.cluster.ID, Kind: ScaleKindIn, Status: ScaleOperationRunning, CreatedBy: 99, CreatedByName: "alice"}
	if err := env.scale.repo.Create(ctx, running); err != nil {
		t.Fatalf("seed running op: %v", err)
	}
	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	_, err := env.scale.SubmitScaleOut(ctx, scaleActor(), env.cluster.ID, &ScaleOutSubmitRequest{ScaleOutRequest: ScaleOutRequest{HostID: 2}, PlanFingerprint: plan.Fingerprint}, ScaleExecutionRequest{})
	if !errors.Is(err, ErrClusterScaleInProgress) {
		t.Fatalf("expected ErrClusterScaleInProgress, got %v", err)
	}
	var inProgress *ScaleInProgressError
	if !errors.As(err, &inProgress) || inProgress.OperationID != running.ID || inProgress.Kind != ScaleKindIn || inProgress.CreatedByName != "alice" {
		t.Fatalf("conflict details = %+v", inProgress)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	writeScaleError(c, err)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d", recorder.Code)
	}
	var body struct {
		ErrorCode string `json:"error_code"`
		ErrorMsg  string `json:"error_msg"`
		Data      struct {
			OperationID   uint   `json:"operation_id"`
			Kind          string `json:"kind"`
			CreatedByName string `json:"created_by_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ErrorCode != ErrorCodeClusterScaleInProgress || body.Data.OperationID != running.ID || body.Data.Kind != string(ScaleKindIn) || body.Data.CreatedByName != "alice" {
		t.Fatalf("409 body = %+v", body)
	}
	if !strings.Contains(body.ErrorMsg, "alice") {
		t.Fatalf("message must name the initiator: %s", body.ErrorMsg)
	}

	// 未记录用户名时退化为用户 ID。
	// Falls back to the user ID when no name was recorded.
	nameless := &ScaleInProgressError{OperationID: 5, Kind: ScaleKindOut, CreatedBy: 42}
	if !strings.Contains(nameless.Error(), "user#42") {
		t.Fatalf("nameless message = %s", nameless.Error())
	}
}

// TestPlanScaleInBlocksSharedProcess 节点进程同时登记在其他集群（同主机、同目录、同角色）时阻断缩容；
// 不同安装目录的节点互不影响。
// TestPlanScaleInBlocksSharedProcess blocks scale-in when the node's process is also registered in another cluster
// (same host, directory and role); nodes in different install directories do not interfere.
func TestPlanScaleInBlocksSharedProcess(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker, 2: NodeRoleMasterWorker})
	target, _ := env.svc.repo.GetNodeByClusterAndHostAndRole(ctx, env.cluster.ID, 2, string(NodeRoleMasterWorker))

	other := &Cluster{Name: "XCEC-" + t.Name(), DeploymentMode: DeploymentModeHybrid, Version: env.cluster.Version, InstallDir: scaleTestInstallDir, Status: ClusterStatusRunning}
	if err := env.svc.repo.Create(ctx, other); err != nil {
		t.Fatalf("create other cluster: %v", err)
	}
	shared := &ClusterNode{ClusterID: other.ID, HostID: 2, Role: NodeRoleMasterWorker, InstallDir: scaleTestInstallDir + "/", HazelcastPort: 5801, APIPort: 8080, Status: NodeStatusRunning}
	if err := env.svc.repo.AddNode(ctx, shared); err != nil {
		t.Fatalf("add shared node: %v", err)
	}

	plan, err := env.scale.PlanScaleIn(ctx, env.cluster.ID, target.ID, "en")
	if err != nil {
		t.Fatalf("PlanScaleIn: %v", err)
	}
	checks := findCheck(plan, "shared_process")
	if len(checks) != 1 || checks[0].Status != ScaleCheckFailed || !checks[0].Blocking || plan.CanSubmit {
		t.Fatalf("shared process must block scale-in: %+v can=%v", checks, plan.CanSubmit)
	}
	if !strings.Contains(checks[0].Target, other.Name) {
		t.Fatalf("the other cluster must be named: %+v", checks[0])
	}

	// 其他集群的节点改用不同安装目录后不再视为共享。
	// A node of another cluster in a different install directory is not shared.
	shared.InstallDir = "/opt/seatunnel-other"
	if err := env.svc.repo.UpdateNode(ctx, shared); err != nil {
		t.Fatalf("update shared node: %v", err)
	}
	plan, err = env.scale.PlanScaleIn(ctx, env.cluster.ID, target.ID, "en")
	if err != nil {
		t.Fatalf("PlanScaleIn: %v", err)
	}
	if len(findCheck(plan, "shared_process")) != 0 {
		t.Fatalf("different install directories must not be shared: %+v", plan.Checks)
	}
}

// TestScaleOutVerifyRejectsIsolatedNewNode 回归：现有节点数据库状态滞后（刚重启）且新节点只看到自己时，
// 入群校验不得误判成功（2026-10-22 t2 实测发现）。
// TestScaleOutVerifyRejectsIsolatedNewNode is a regression test: when an existing node's DB status lags (just
// restarted) and the new node only sees itself, the join check must not falsely succeed (found on t2, 2026-10-22).
func TestScaleOutVerifyRejectsIsolatedNewNode(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	env.agent.onCommand = func(agentID, commandType string, params map[string]string) {
		if commandType == "start" && agentID == "agent-2" && params["service"] == "" {
			// 新节点自成集群；现有节点刚重启，探测时进程暂不可见（会被刷成 stopped），复现现场。
			// The new node forms its own cluster; the existing node was just restarted and its process is briefly
			// invisible to detection (it gets refreshed to stopped), reproducing the field case.
			env.agent.setMembers([]string{"10.0.0.2:5801"}, "agent-2")
			env.agent.running["agent-1|"+scaleTestInstallDir] = false
		}
	}

	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	result, err := env.scale.SubmitScaleOut(ctx, scaleActor(), env.cluster.ID,
		&ScaleOutSubmitRequest{ScaleOutRequest: ScaleOutRequest{HostID: 2}, PlanFingerprint: plan.Fingerprint}, ScaleExecutionRequest{})
	if err != nil {
		t.Fatalf("SubmitScaleOut: %v", err)
	}
	op := env.loadOp(t, result.OperationID)
	if op.Status != ScaleOperationFailed || op.CurrentStep != ScaleStepVerifyMembership {
		t.Fatalf("an isolated new node must fail verify_membership, got %s at %s", op.Status, op.CurrentStep)
	}
	if step := stepByName(op, ScaleStepVerifyMembership); !strings.Contains(step.Error, "1/2") {
		t.Fatalf("the error must report 1 of 2 expected members: %s", step.Error)
	}
}
