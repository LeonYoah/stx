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
	"net/http"
	"testing"

	appconfig "github.com/LeonYoah/stx/internal/apps/config"
)

// TestPlanScaleOutRollingRestartCandidates 滚动重启候选：运行中的现有节点；回环节点（修正时已重启）不在其中；
// 有运行中作业时不可选；作业未知时可选并给出说明。
// TestPlanScaleOutRollingRestartCandidates: candidates are the running existing nodes; loopback nodes (restarted by
// the fix) are excluded; running jobs make it unavailable; unknown jobs keep it available with a note.
func TestPlanScaleOutRollingRestartCandidates(t *testing.T) {
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker, 2: NodeRoleMasterWorker})
	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 3})
	restart := plan.RollingRestart
	if restart == nil || len(restart.Nodes) != 2 || !restart.Available || !restart.Recommended || restart.Note != "" {
		t.Fatalf("two running existing nodes should be recommended candidates: %+v", restart)
	}

	env.jobs.err = errors.New("engine http disabled")
	unknown := env.mustPlanOut(t, ScaleOutRequest{HostID: 3}).RollingRestart
	if !unknown.Available || unknown.Note == "" {
		t.Fatalf("unknown jobs keep it available with a note: %+v", unknown)
	}

	env.jobs.err = nil
	env.jobs.jobs = []ScaleRunningJob{{JobID: "1", JobName: "cdc"}}
	blocked := env.mustPlanOut(t, ScaleOutRequest{HostID: 3}).RollingRestart
	if blocked.Available || blocked.Recommended || blocked.BlockedReason == "" {
		t.Fatalf("running jobs must make it unavailable: %+v", blocked)
	}

	// 回环节点由回环修正重启并写入完整列表，不再作为候选。
	// Loopback nodes are restarted by the loopback fix with the full list and are not candidates.
	loopEnv := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	hostID := uint(1)
	loopEnv.config.configs = append(loopEnv.config.configs, &appconfig.ConfigInfo{ID: 10, ClusterID: loopEnv.cluster.ID, HostID: &hostID, ConfigType: appconfig.ConfigTypeHazelcast, Content: hazelcastMembers("127.0.0.1:5801")})
	loopPlan := loopEnv.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	if len(loopPlan.LoopbackNodes) != 1 || len(loopPlan.RollingRestart.Nodes) != 0 || loopPlan.RollingRestart.Available {
		t.Fatalf("a loopback node must not be a rolling-restart candidate: %+v", loopPlan.RollingRestart)
	}
}

// TestSubmitRollingRestartRejectedWhenUnavailable 计划不允许时选择滚动重启返回 400。
// TestSubmitRollingRestartRejectedWhenUnavailable: choosing a rolling restart the plan does not allow returns 400.
func TestSubmitRollingRestartRejectedWhenUnavailable(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	env.jobs.jobs = []ScaleRunningJob{{JobID: "1"}}
	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	_, err := env.scale.SubmitScaleOut(ctx, scaleActor(), env.cluster.ID,
		&ScaleOutSubmitRequest{ScaleOutRequest: ScaleOutRequest{HostID: 2}, PlanFingerprint: plan.Fingerprint, RollingRestart: true}, ScaleExecutionRequest{})
	if !errors.Is(err, ErrScaleRollingRestartUnavailable) {
		t.Fatalf("expected ErrScaleRollingRestartUnavailable, got %v", err)
	}
	if status, code := scaleErrorStatus(err); status != http.StatusBadRequest || code != ErrorCodeScaleRollingRestartUnavailable {
		t.Fatalf("expected 400 %s, got %d %s", ErrorCodeScaleRollingRestartUnavailable, status, code)
	}
}

// TestScaleOutRollingRestartRestartsExistingNodesAfterJoin 选择滚动重启时：入群校验之后逐个重启现有节点，
// 新节点不重启，每个节点重启后需看到全部成员。
// TestScaleOutRollingRestartRestartsExistingNodesAfterJoin: with a rolling restart, existing nodes restart one by one
// after the join is verified; the new node is not restarted, and each node must see every member afterwards.
func TestScaleOutRollingRestartRestartsExistingNodesAfterJoin(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker, 2: NodeRoleMasterWorker})
	all := []string{"10.0.0.1:5801", "10.0.0.2:5801", "10.0.0.3:5801"}
	env.joinOnStart("agent-3", all)

	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 3})
	result, err := env.scale.SubmitScaleOut(ctx, scaleActor(), env.cluster.ID,
		&ScaleOutSubmitRequest{ScaleOutRequest: ScaleOutRequest{HostID: 3}, PlanFingerprint: plan.Fingerprint, RollingRestart: true}, ScaleExecutionRequest{})
	if err != nil {
		t.Fatalf("SubmitScaleOut: %v", err)
	}
	op := env.loadOp(t, result.OperationID)
	if op.Status != ScaleOperationSucceeded || !op.RollingRestart {
		t.Fatalf("operation should succeed with rolling restart, got %s: %s", op.Status, op.Error)
	}
	steps := op.steps()
	if last := steps[len(steps)-1]; last.Name != ScaleStepRollingRestart || last.Status != ScaleStepSucceeded {
		t.Fatalf("rolling_restart must be the last step and succeed: %+v", last)
	}
	for _, agent := range []string{"agent-1", "agent-2"} {
		if env.agent.count(agent, "restart") != 1 {
			t.Fatalf("%s must be restarted exactly once, got %d", agent, env.agent.count(agent, "restart"))
		}
	}
	if env.agent.count("agent-3", "restart") != 0 {
		t.Fatal("the new node must not be restarted")
	}

	// 不选滚动重启时没有该步骤。/ Without the option there is no such step.
	env2 := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	env2.joinOnStart("agent-2", []string{"10.0.0.1:5801", "10.0.0.2:5801"})
	plan2 := env2.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	result2, err := env2.scale.SubmitScaleOut(ctx, scaleActor(), env2.cluster.ID,
		&ScaleOutSubmitRequest{ScaleOutRequest: ScaleOutRequest{HostID: 2}, PlanFingerprint: plan2.Fingerprint}, ScaleExecutionRequest{})
	if err != nil {
		t.Fatalf("SubmitScaleOut: %v", err)
	}
	for _, step := range env2.loadOp(t, result2.OperationID).steps() {
		if step.Name == ScaleStepRollingRestart {
			t.Fatal("rolling_restart must not run unless chosen")
		}
	}
	if env2.agent.count("agent-1", "restart") != 0 {
		t.Fatal("existing nodes must not restart without the option")
	}
}

// TestScaleOutRollingRestartStopsWhenJobsAppear 滚动重启前复核作业：出现运行中作业即停止，不再重启。
// TestScaleOutRollingRestartStopsWhenJobsAppear: jobs are re-checked before each restart; appearing jobs stop it.
func TestScaleOutRollingRestartStopsWhenJobsAppear(t *testing.T) {
	ctx := context.Background()
	env := newScaleTestEnv(t, DeploymentModeHybrid, map[uint]NodeRole{1: NodeRoleMasterWorker})
	env.agent.onCommand = func(agentID, commandType string, params map[string]string) {
		if commandType == "start" && agentID == "agent-2" && params["service"] == "" {
			env.agent.setMembers([]string{"10.0.0.1:5801", "10.0.0.2:5801"}, "agent-1", "agent-2")
			// 入群后、滚动重启前出现运行中作业。/ A job appears after the join, before the rolling restart.
			env.jobs.jobs = []ScaleRunningJob{{JobID: "9", JobName: "late"}}
		}
	}
	plan := env.mustPlanOut(t, ScaleOutRequest{HostID: 2})
	result, err := env.scale.SubmitScaleOut(ctx, scaleActor(), env.cluster.ID,
		&ScaleOutSubmitRequest{ScaleOutRequest: ScaleOutRequest{HostID: 2}, PlanFingerprint: plan.Fingerprint, RollingRestart: true}, ScaleExecutionRequest{})
	if err != nil {
		t.Fatalf("SubmitScaleOut: %v", err)
	}
	op := env.loadOp(t, result.OperationID)
	if op.Status != ScaleOperationFailed || op.CurrentStep != ScaleStepRollingRestart {
		t.Fatalf("rolling restart must stop on running jobs, got %s at %s", op.Status, op.CurrentStep)
	}
	if env.agent.count("agent-1", "restart") != 0 {
		t.Fatal("no node may be restarted once jobs are running")
	}
	if step := stepByName(op, ScaleStepVerifyMembership); step.Status != ScaleStepSucceeded {
		t.Fatalf("the join itself must still have succeeded: %+v", step)
	}
}
