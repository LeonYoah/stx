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

package stupgrade

import (
	"context"
	"strings"
	"testing"
)

func countSubCommandInDir(commands []agentCommandRecord, subCommand, installDir string) int {
	count := 0
	for _, command := range commands {
		if command.params["sub_command"] == subCommand && (installDir == "" || command.params["install_dir"] == installDir) {
			count++
		}
	}
	return count
}

func findStep(task *UpgradeTask, code StepCode) *UpgradeTaskStep {
	for i := range task.Steps {
		if task.Steps[i].Code == code {
			return &task.Steps[i]
		}
	}
	return nil
}

// 健康检查必须等到目标目录的 SeaTunnel 进程真正运行（异步拉起）。
// The health check must wait until the SeaTunnel process of the target dir is really running (async start).
func TestService_ExecutePlan_healthCheckWaitsForProcess(t *testing.T) {
	repo := NewRepository(openTestDB(t))
	clusterOperator := &stubClusterOperator{}
	agentSender := &stubAgentCommandSender{
		agents:            map[uint]string{101: "agent-node-a"},
		processDownChecks: map[string]int{"/opt/seatunnel-2.3.12": 2},
	}
	service := newExecutionService(t, repo, clusterOperator, agentSender)

	task, err := service.ExecutePlan(context.Background(), mustCreateReadyPlan(t, service), 7)
	if err != nil {
		t.Fatalf("ExecutePlan returned error: %v", err)
	}
	if task.Status != ExecutionStatusSucceeded {
		t.Fatalf("expected success once the process comes up, got %s (%s)", task.Status, task.FailureReason)
	}
	if got := countSubCommandInDir(agentSender.commands, "check_process", "/opt/seatunnel-2.3.12"); got != 3 {
		t.Fatalf("expected 3 check_process probes on the target dir, got %d", got)
	}
}

// 进程始终未运行时，健康检查失败并触发回滚，回滚校验同样要求源目录进程运行。
// When the process never runs the health check fails and triggers rollback; rollback verify also
// requires the source-dir process to run.
func TestService_ExecutePlan_healthCheckFailsWhenProcessNeverRuns(t *testing.T) {
	repo := NewRepository(openTestDB(t))
	clusterOperator := &stubClusterOperator{}
	agentSender := &stubAgentCommandSender{
		agents:            map[uint]string{101: "agent-node-a"},
		processDownChecks: map[string]int{"/opt/seatunnel-2.3.12": -1},
	}
	service := newExecutionService(t, repo, clusterOperator, agentSender)

	task, err := service.ExecutePlan(context.Background(), mustCreateReadyPlan(t, service), 7)
	if err != nil {
		t.Fatalf("ExecutePlan returned error: %v", err)
	}
	if task.Status != ExecutionStatusFailed || task.FailureStep != StepCodeHealthCheck {
		t.Fatalf("expected HEALTH_CHECK failure, got status=%s step=%s", task.Status, task.FailureStep)
	}
	if !strings.Contains(task.FailureReason, "进程未运行") && !strings.Contains(task.FailureReason, "not running") {
		t.Fatalf("failure reason should mention the process, got %q", task.FailureReason)
	}
	if task.RollbackStatus != ExecutionStatusRollbackSucceeded {
		t.Fatalf("expected rollback to succeed, got %s", task.RollbackStatus)
	}
	if countSubCommandInDir(agentSender.commands, "check_process", "/opt/seatunnel-2.3.11") == 0 {
		t.Fatalf("rollback verify must probe the source-dir process, got %+v", agentSender.commands)
	}
}

// 切换窗口之前失败：不停止集群、不重启，只校验源目录完好。
// Failure before the cutover window: the cluster is not stopped or restarted; only the source dir is verified.
func TestService_ExecutePlan_failureBeforeCutoverKeepsClusterRunning(t *testing.T) {
	repo := NewRepository(openTestDB(t))
	clusterOperator := &stubClusterOperator{}
	agentSender := &stubAgentCommandSender{
		agents:         map[uint]string{101: "agent-node-a"},
		failSubCommand: "apply_merged_config",
	}
	service := newExecutionService(t, repo, clusterOperator, agentSender)

	task, err := service.ExecutePlan(context.Background(), mustCreateReadyPlan(t, service), 7)
	if err != nil {
		t.Fatalf("ExecutePlan returned error: %v", err)
	}
	if task.Status != ExecutionStatusFailed || task.FailureStep != StepCodeMergeConfig {
		t.Fatalf("expected MERGE_CONFIG failure, got status=%s step=%s reason=%s", task.Status, task.FailureStep, task.FailureReason)
	}
	if task.RollbackStatus != ExecutionStatusRollbackSucceeded {
		t.Fatalf("expected rollback_succeeded, got %s", task.RollbackStatus)
	}
	if clusterOperator.stopCalls != 0 || clusterOperator.startCalls != 0 || clusterOperator.proxyStopCalls != 0 {
		t.Fatalf("cluster must not be stopped/started before cutover, got stop=%d start=%d proxyStop=%d", clusterOperator.stopCalls, clusterOperator.startCalls, clusterOperator.proxyStopCalls)
	}
	if step := findStep(task, StepCodeRollbackRestart); step == nil || step.Status != ExecutionStatusSkipped {
		t.Fatalf("expected ROLLBACK_RESTART skipped, got %+v", step)
	}
	if step := findStep(task, StepCodeRollbackVerify); step == nil || step.Status != ExecutionStatusSucceeded {
		t.Fatalf("expected ROLLBACK_VERIFY succeeded, got %+v", step)
	}
	if countSubCommandInDir(agentSender.commands, "verify_cluster_health", "/opt/seatunnel-2.3.11") == 0 {
		t.Fatalf("expected the source dir layout to be verified")
	}
	if len(task.NodeExecutions) != 1 || task.NodeExecutions[0].Status != ExecutionStatusRollbackSucceeded {
		t.Fatalf("expected node rollback_succeeded, got %+v", task.NodeExecutions)
	}
}

func TestCutoverWindowEntered(t *testing.T) {
	steps := []UpgradeTaskStep{{Code: StepCodeMergeConfig, Status: ExecutionStatusFailed}, {Code: StepCodeStopCluster, Status: ExecutionStatusPending}}
	if cutoverWindowEntered(steps) {
		t.Fatal("pending STOP_CLUSTER means the cutover window was not entered")
	}
	steps[1].Status = ExecutionStatusFailed
	if !cutoverWindowEntered(steps) {
		t.Fatal("a failed STOP_CLUSTER may have stopped some nodes, so the window counts as entered")
	}
	if sourceDirModified(map[string]string{"node-a": ""}) || !sourceDirModified(map[string]string{"node-a": "/backup/a"}) {
		t.Fatal("sourceDirModified should only report non-empty backup paths")
	}
}
