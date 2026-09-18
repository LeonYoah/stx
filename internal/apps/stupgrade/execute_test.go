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
	"fmt"
	"strings"
	"testing"
	"time"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	pluginapp "github.com/LeonYoah/stx/internal/apps/plugin"
)

type stubClusterOperator struct {
	stopCalls         int
	startCalls        int
	clusterVersions   []string
	clusterInstallDir []string
	nodeInstallDirs   map[uint][]string
}

func (s *stubClusterOperator) Start(ctx context.Context, clusterID uint) (*clusterapp.OperationResult, error) {
	s.startCalls++
	return &clusterapp.OperationResult{
		ClusterID: clusterID,
		Operation: clusterapp.OperationStart,
		Success:   true,
		Message:   "cluster started",
		NodeResults: []*clusterapp.NodeOperationResult{{
			NodeID:   11,
			HostID:   101,
			HostName: "node-a",
			Success:  true,
			Message:  "started",
		}},
	}, nil
}

func (s *stubClusterOperator) Stop(ctx context.Context, clusterID uint) (*clusterapp.OperationResult, error) {
	s.stopCalls++
	return &clusterapp.OperationResult{
		ClusterID: clusterID,
		Operation: clusterapp.OperationStop,
		Success:   true,
		Message:   "cluster stopped",
		NodeResults: []*clusterapp.NodeOperationResult{{
			NodeID:   11,
			HostID:   101,
			HostName: "node-a",
			Success:  true,
			Message:  "stopped",
		}},
	}, nil
}

func (s *stubClusterOperator) Update(ctx context.Context, id uint, req *clusterapp.UpdateClusterRequest) (*clusterapp.Cluster, error) {
	if req.Version != nil {
		s.clusterVersions = append(s.clusterVersions, *req.Version)
	}
	if req.InstallDir != nil {
		s.clusterInstallDir = append(s.clusterInstallDir, *req.InstallDir)
	}
	return &clusterapp.Cluster{ID: id, Version: derefString(req.Version), InstallDir: derefString(req.InstallDir)}, nil
}

func (s *stubClusterOperator) UpdateNode(ctx context.Context, clusterID uint, nodeID uint, req *clusterapp.UpdateNodeRequest) (*clusterapp.ClusterNode, error) {
	if s.nodeInstallDirs == nil {
		s.nodeInstallDirs = make(map[uint][]string)
	}
	if req.InstallDir != nil {
		s.nodeInstallDirs[nodeID] = append(s.nodeInstallDirs[nodeID], *req.InstallDir)
	}
	return &clusterapp.ClusterNode{ID: nodeID, ClusterID: clusterID, InstallDir: derefString(req.InstallDir)}, nil
}

type stubPackageTransferer struct {
	remotePath string
	calls      []string
}

func (s *stubPackageTransferer) TransferPackageToAgent(ctx context.Context, agentID string, version string, status *installerapp.InstallationStatus) (string, error) {
	s.calls = append(s.calls, fmt.Sprintf("%s:%s", agentID, version))
	if s.remotePath != "" {
		return s.remotePath, nil
	}
	return fmt.Sprintf("/tmp/%s/apache-seatunnel-%s-bin.tar.gz", agentID, version), nil
}

type agentCommandRecord struct {
	agentID     string
	commandType string
	params      map[string]string
}

type stubAgentCommandSender struct {
	agents               map[uint]string
	commands             []agentCommandRecord
	failTargetHealthOnce bool
	failedInstallDir     string
	failSmokeTestOnce    bool
	smokeFailureHostID   uint
}

func (s *stubAgentCommandSender) GetAgentByHostID(hostID uint) (string, bool) {
	agentID, ok := s.agents[hostID]
	return agentID, ok
}

func (s *stubAgentCommandSender) SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error) {
	copied := make(map[string]string, len(params))
	for key, value := range params {
		copied[key] = value
	}
	s.commands = append(s.commands, agentCommandRecord{agentID: agentID, commandType: commandType, params: copied})

	subCommand := copied["sub_command"]
	switch subCommand {
	case "backup_install_dir":
		return true, fmt.Sprintf("backup_path=%s", copied["backup_dir"]), nil
	case "extract_package_to_dir":
		return true, fmt.Sprintf("target_dir=%s", copied["target_dir"]), nil
	case "sync_connectors_manifest":
		return true, fmt.Sprintf("connectors_dir=%s/connectors", copied["install_dir"]), nil
	case "sync_lib_manifest":
		return true, fmt.Sprintf("lib_dir=%s/lib", copied["install_dir"]), nil
	case "sync_plugins_manifest":
		return true, fmt.Sprintf("plugins_dir=%s/plugins", copied["install_dir"]), nil
	case "apply_merged_config":
		return true, `{"success":true}`, nil
	case "switch_install_dir":
		return true, fmt.Sprintf("switched_path=%s", copied["target_dir"]), nil
	case "restore_snapshot":
		return true, fmt.Sprintf("restore_dir=%s", copied["restore_dir"]), nil
	case "verify_cluster_health":
		if s.failTargetHealthOnce && strings.Contains(copied["install_dir"], s.failedInstallDir) {
			s.failTargetHealthOnce = false
			return false, "health check failed", nil
		}
		return true, "health_ok", nil
	case "run_smoke_test_template":
		if s.failSmokeTestOnce && (s.smokeFailureHostID == 0 || s.smokeFailureHostID == copiedHostID(agentID, s.agents)) {
			s.failSmokeTestOnce = false
			return false, "template smoke test failed", nil
		}
		return true, "smoke_ok", nil
	default:
		return false, fmt.Sprintf("unsupported sub_command: %s", subCommand), nil
	}
}

func TestService_ExecutePlan_success(t *testing.T) {
	database := openTestDB(t)
	repo := NewRepository(database)
	clusterOperator := &stubClusterOperator{}
	agentSender := &stubAgentCommandSender{agents: map[uint]string{101: "agent-node-a"}}
	service := newExecutionService(t, repo, clusterOperator, agentSender)
	pluginProvider := &stubPluginProvider{
		installed: []pluginapp.InstalledPlugin{{
			ClusterID:  1,
			PluginName: "jdbc",
			Version:    "2.3.11",
		}},
		local: []pluginapp.LocalPlugin{{
			Name:       "jdbc",
			ArtifactID: "connector-jdbc",
			Version:    "2.3.12",
		}},
	}
	service.SetPluginProvider(pluginProvider)

	planID := mustCreateReadyPlan(t, service)
	task, err := service.ExecutePlan(context.Background(), planID, 7)
	if err != nil {
		t.Fatalf("ExecutePlan returned error: %v", err)
	}
	if task.Status != ExecutionStatusSucceeded {
		t.Fatalf("expected task status succeeded, got %s", task.Status)
	}
	if task.RollbackStatus != ExecutionStatusPending {
		t.Fatalf("expected rollback status pending, got %s", task.RollbackStatus)
	}
	if task.CurrentStep != StepCodeComplete {
		t.Fatalf("expected current step COMPLETE, got %s", task.CurrentStep)
	}
	if len(task.Steps) != len(DefaultExecutionSteps())+len(DefaultRollbackSteps(len(DefaultExecutionSteps())+1)) {
		t.Fatalf("expected task steps to include rollback branch, got %d", len(task.Steps))
	}
	if len(task.NodeExecutions) != 1 {
		t.Fatalf("expected 1 node execution, got %d", len(task.NodeExecutions))
	}
	node := task.NodeExecutions[0]
	if node.Status != ExecutionStatusSucceeded {
		t.Fatalf("expected node status succeeded, got %s", node.Status)
	}
	if node.CurrentStep != StepCodeComplete {
		t.Fatalf("expected node current step COMPLETE, got %s", node.CurrentStep)
	}
	if clusterOperator.stopCalls != 1 || clusterOperator.startCalls != 1 {
		t.Fatalf("expected stop/start to each run once, got stop=%d start=%d", clusterOperator.stopCalls, clusterOperator.startCalls)
	}
	if len(clusterOperator.clusterVersions) == 0 || clusterOperator.clusterVersions[len(clusterOperator.clusterVersions)-1] != "2.3.12" {
		t.Fatalf("expected cluster version updated to 2.3.12, got %+v", clusterOperator.clusterVersions)
	}
	if pluginProvider.recorded["jdbc"] != "2.3.12" {
		t.Fatalf("expected installed plugin version reconciled to 2.3.12, got %+v", pluginProvider.recorded)
	}
	if got := lastString(clusterOperator.nodeInstallDirs[11]); got != "/opt/seatunnel-2.3.12" {
		t.Fatalf("expected node install dir switched to target version, got %q", got)
	}
	for _, command := range agentSender.commands {
		if command.commandType != "upgrade" {
			t.Fatalf("expected managed upgrade command type, got %s", command.commandType)
		}
		if strings.TrimSpace(command.params["sub_command"]) == "" {
			t.Fatalf("expected all commands to use managed sub_command, got %+v", command.params)
		}
	}
	if hasSubCommand(agentSender.commands, "backup_install_dir") {
		t.Fatalf("expected double-directory upgrade to skip backup_install_dir command, got %+v", agentSender.commands)
	}
	if !hasSubCommand(agentSender.commands, "run_smoke_test_template") {
		t.Fatalf("expected managed smoke test command, got %+v", agentSender.commands)
	}
	logs, total, err := service.ListStepLogs(context.Background(), &StepLogFilter{TaskID: task.ID, Page: 1, PageSize: 200})
	if err != nil {
		t.Fatalf("ListStepLogs returned error: %v", err)
	}
	if total == 0 || len(logs) == 0 {
		t.Fatalf("expected persisted logs, got total=%d len=%d", total, len(logs))
	}
	foundNodeCommandLog := false
	for _, logEntry := range logs {
		if logEntry.NodeExecutionID != nil && strings.TrimSpace(logEntry.CommandSummary) != "" {
			foundNodeCommandLog = true
			break
		}
	}
	if !foundNodeCommandLog {
		t.Fatalf("expected node-level command summary logs, got %+v", logs)
	}
}

func TestService_ExecutePlan_sameInstallDirTriggersBackup(t *testing.T) {
	database := openTestDB(t)
	repo := NewRepository(database)
	clusterOperator := &stubClusterOperator{}
	agentSender := &stubAgentCommandSender{agents: map[uint]string{101: "agent-node-a"}}
	service := newExecutionService(t, repo, clusterOperator, agentSender)

	planID := mustCreateReadyPlan(t, service)
	plan, err := service.GetPlan(context.Background(), planID)
	if err != nil {
		t.Fatalf("GetPlan returned error: %v", err)
	}
	plan.Snapshot.NodeTargets[0].TargetInstallDir = plan.Snapshot.NodeTargets[0].SourceInstallDir
	if err := service.UpdatePlan(context.Background(), plan); err != nil {
		t.Fatalf("UpdatePlan returned error: %v", err)
	}

	task, err := service.ExecutePlan(context.Background(), planID, 7)
	if err != nil {
		t.Fatalf("ExecutePlan returned error: %v", err)
	}
	if task.Status != ExecutionStatusSucceeded {
		t.Fatalf("expected task status succeeded, got %s", task.Status)
	}
	if !hasSubCommand(agentSender.commands, "backup_install_dir") {
		t.Fatalf("expected same-directory upgrade to trigger backup_install_dir, got %+v", agentSender.commands)
	}
	for _, command := range agentSender.commands {
		if command.params["sub_command"] != "backup_install_dir" {
			continue
		}
		if strings.TrimSpace(command.params["backup_dir"]) == "" {
			t.Fatalf("expected backup_dir to be set for same-directory backup, got %+v", command.params)
		}
	}
}

func TestService_ListTasks_filtersAndPaginates(t *testing.T) {
	database := openTestDB(t)
	repo := NewRepository(database)
	service := NewService(repo)

	buildPlan := func(clusterID uint, targetVersion string) *UpgradePlanRecord {
		t.Helper()
		plan, err := service.CreatePlan(context.Background(), UpgradePlanSnapshot{
			ClusterID:     clusterID,
			SourceVersion: "2.3.11",
			TargetVersion: targetVersion,
			PackageManifest: PackageManifest{
				Version: targetVersion,
			},
			ConnectorManifest: ConnectorManifest{
				Version:    targetVersion,
				Connectors: []ConnectorArtifact{},
				Libraries:  []LibraryArtifact{},
			},
			ConfigMergePlan: ConfigMergePlan{
				Ready:       true,
				Files:       []ConfigMergeFile{},
				GeneratedAt: time.Now(),
			},
			NodeTargets: []NodeTarget{},
			GeneratedAt: time.Now(),
		}, 8, PlanStatusReady, 0)
		if err != nil {
			t.Fatalf("CreatePlan returned error: %v", err)
		}
		return plan
	}

	firstPlan := buildPlan(201, "2.3.12")
	firstTask, err := service.CreateTaskFromPlan(context.Background(), firstPlan.ID, 7)
	if err != nil {
		t.Fatalf("CreateTaskFromPlan returned error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	secondPlan := buildPlan(201, "2.3.13")
	secondTask, err := service.CreateTaskFromPlan(context.Background(), secondPlan.ID, 8)
	if err != nil {
		t.Fatalf("CreateTaskFromPlan for second task returned error: %v", err)
	}
	thirdPlan := buildPlan(202, "2.3.14")
	if _, err := service.CreateTaskFromPlan(context.Background(), thirdPlan.ID, 9); err != nil {
		t.Fatalf("CreateTaskFromPlan for third task returned error: %v", err)
	}

	filtered, total, err := service.ListTasks(context.Background(), &TaskListFilter{
		ClusterID: 201,
		Page:      1,
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("ListTasks returned error: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total=2 after cluster filter, got %d", total)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected filtered list length=2, got %d", len(filtered))
	}
	if filtered[0].ID != secondTask.ID || filtered[1].ID != firstTask.ID {
		t.Fatalf("expected filtered list to contain only cluster 201 tasks in desc order, got %+v", filtered)
	}

	paged, total, err := service.ListTasks(context.Background(), &TaskListFilter{
		ClusterID: 201,
		Page:      1,
		PageSize:  1,
	})
	if err != nil {
		t.Fatalf("ListTasks with pagination returned error: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total=2 for all tasks, got %d", total)
	}
	if len(paged) != 1 {
		t.Fatalf("expected paged result length=1, got %d", len(paged))
	}
	if paged[0].ID != secondTask.ID {
		t.Fatalf("expected latest task to be returned first, got %+v", paged[0])
	}
}

func TestService_ExecutePlan_failureTriggersRollback(t *testing.T) {
	database := openTestDB(t)
	repo := NewRepository(database)
	clusterOperator := &stubClusterOperator{}
	agentSender := &stubAgentCommandSender{
		agents:               map[uint]string{101: "agent-node-a"},
		failTargetHealthOnce: true,
		failedInstallDir:     "/opt/seatunnel-2.3.12",
	}
	service := newExecutionService(t, repo, clusterOperator, agentSender)

	planID := mustCreateReadyPlan(t, service)
	task, err := service.ExecutePlan(context.Background(), planID, 7)
	if err != nil {
		t.Fatalf("ExecutePlan returned error: %v", err)
	}
	if task.Status != ExecutionStatusFailed {
		t.Fatalf("expected task status failed, got %s", task.Status)
	}
	if task.RollbackStatus != ExecutionStatusRollbackSucceeded {
		t.Fatalf("expected rollback status rollback_succeeded, got %s", task.RollbackStatus)
	}
	if task.FailureStep != StepCodeHealthCheck {
		t.Fatalf("expected failure step HEALTH_CHECK, got %s", task.FailureStep)
	}
	if !strings.Contains(task.FailureReason, "health check failed") {
		t.Fatalf("expected failure reason to contain health check failed, got %q", task.FailureReason)
	}
	if clusterOperator.stopCalls != 2 || clusterOperator.startCalls != 2 {
		t.Fatalf("expected upgrade + rollback to call stop/start twice, got stop=%d start=%d", clusterOperator.stopCalls, clusterOperator.startCalls)
	}
	if got := lastString(clusterOperator.clusterVersions); got != "2.3.11" {
		t.Fatalf("expected cluster version restored to source version, got %q", got)
	}
	if got := lastString(clusterOperator.nodeInstallDirs[11]); got != "/opt/seatunnel-2.3.11" {
		t.Fatalf("expected node install dir restored to source version, got %q", got)
	}
	if len(task.NodeExecutions) != 1 || task.NodeExecutions[0].Status != ExecutionStatusRollbackSucceeded {
		t.Fatalf("expected node execution rollback_succeeded, got %+v", task.NodeExecutions)
	}
	if hasSubCommand(agentSender.commands, "restore_snapshot") {
		t.Fatalf("expected rollback to skip restore_snapshot when source dir stays intact, got %+v", agentSender.commands)
	}
	var rollbackVerifyStep *UpgradeTaskStep
	var failedClosureStep *UpgradeTaskStep
	for i := range task.Steps {
		step := &task.Steps[i]
		switch step.Code {
		case StepCodeRollbackVerify:
			rollbackVerifyStep = step
		case StepCodeFailed:
			failedClosureStep = step
		}
	}
	if rollbackVerifyStep == nil || rollbackVerifyStep.Status != ExecutionStatusSucceeded {
		t.Fatalf("expected rollback verify step succeeded, got %+v", rollbackVerifyStep)
	}
	if failedClosureStep == nil || failedClosureStep.Status != ExecutionStatusSucceeded {
		t.Fatalf("expected FAILED closure step succeeded, got %+v", failedClosureStep)
	}
	logs, total, err := service.ListStepLogs(context.Background(), &StepLogFilter{TaskID: task.ID, Page: 1, PageSize: 200})
	if err != nil {
		t.Fatalf("ListStepLogs returned error: %v", err)
	}
	if total == 0 {
		t.Fatalf("expected rollback logs to be persisted")
	}
	foundRollbackLog := false
	for _, logEntry := range logs {
		if logEntry.StepCode == StepCodeRollbackPrepare || logEntry.StepCode == StepCodeRollbackVerify {
			foundRollbackLog = true
			break
		}
	}
	if !foundRollbackLog {
		t.Fatalf("expected rollback logs to be present, got %+v", logs)
	}
}

func TestService_ExecutePlan_syncsPluginsManifestWhenPresent(t *testing.T) {
	database := openTestDB(t)
	repo := NewRepository(database)
	clusterOperator := &stubClusterOperator{}
	agentSender := &stubAgentCommandSender{agents: map[uint]string{101: "agent-node-a"}}
	service := newExecutionService(t, repo, clusterOperator, agentSender)

	plan, err := service.CreatePlan(context.Background(), UpgradePlanSnapshot{
		ClusterID:     1,
		SourceVersion: "2.3.11",
		TargetVersion: "2.3.12",
		GeneratedAt:   time.Now(),
		PackageManifest: PackageManifest{
			Version:   "2.3.12",
			FileName:  "apache-seatunnel-2.3.12-bin.tar.gz",
			LocalPath: "/tmp/apache-seatunnel-2.3.12-bin.tar.gz",
			Checksum:  "abc123",
			Source:    AssetSourceLocalPackage,
		},
		ConnectorManifest: ConnectorManifest{
			Version:         "2.3.12",
			ReplacementMode: "package_overlay",
			Connectors:      []ConnectorArtifact{},
			Libraries:       []LibraryArtifact{},
			PluginDeps: []PluginDependencyArtifact{{
				GroupID:      "org.example",
				ArtifactID:   "oracle-extra",
				Version:      "2.0.0",
				FileName:     "oracle-extra-2.0.0.jar",
				Source:       AssetSourceLocalPlugin,
				TargetDir:    "plugins/connector-jdbc",
				RelativePath: "connector-jdbc/oracle-extra-2.0.0.jar",
			}},
		},
		ConfigMergePlan: ConfigMergePlan{
			Ready:       true,
			Files:       []ConfigMergeFile{},
			GeneratedAt: time.Now(),
		},
		NodeTargets: []NodeTarget{{
			ClusterNodeID:    11,
			HostID:           101,
			HostName:         "node-a",
			HostIP:           "10.0.0.1",
			Role:             string(clusterapp.NodeRoleMasterWorker),
			Arch:             "amd64",
			SourceVersion:    "2.3.11",
			TargetVersion:    "2.3.12",
			SourceInstallDir: "/opt/seatunnel-2.3.11",
			TargetInstallDir: "/opt/seatunnel-2.3.12",
		}},
		Steps: DefaultExecutionSteps(),
	}, 7, PlanStatusReady, 0)
	if err != nil {
		t.Fatalf("CreatePlan returned error: %v", err)
	}

	task, err := service.ExecutePlan(context.Background(), plan.ID, 7)
	if err != nil {
		t.Fatalf("ExecutePlan returned error: %v", err)
	}
	if task.Status != ExecutionStatusSucceeded {
		t.Fatalf("expected task status succeeded, got %s", task.Status)
	}
	if !hasSubCommand(agentSender.commands, "sync_plugins_manifest") {
		t.Fatalf("expected sync_plugins_manifest command, got %+v", agentSender.commands)
	}
}

func TestService_ExecutePlan_smokeTestWarningDoesNotBlock(t *testing.T) {
	database := openTestDB(t)
	repo := NewRepository(database)
	clusterOperator := &stubClusterOperator{}
	agentSender := &stubAgentCommandSender{
		agents:            map[uint]string{101: "agent-node-a"},
		failSmokeTestOnce: true,
	}
	service := newExecutionService(t, repo, clusterOperator, agentSender)

	planID := mustCreateReadyPlan(t, service)
	task, err := service.ExecutePlan(context.Background(), planID, 7)
	if err != nil {
		t.Fatalf("ExecutePlan returned error: %v", err)
	}
	if task.Status != ExecutionStatusSucceeded {
		t.Fatalf("expected task status succeeded, got %s", task.Status)
	}
	if task.RollbackStatus != ExecutionStatusPending {
		t.Fatalf("expected rollback status pending, got %s", task.RollbackStatus)
	}

	var smokeStep *UpgradeTaskStep
	for i := range task.Steps {
		if task.Steps[i].Code == StepCodeSmokeTest {
			smokeStep = &task.Steps[i]
			break
		}
	}
	if smokeStep == nil {
		t.Fatal("expected smoke test step to exist")
	}
	if smokeStep.Status != ExecutionStatusSucceeded {
		t.Fatalf("expected smoke step succeeded, got %s", smokeStep.Status)
	}
	if !strings.Contains(smokeStep.Message, "告警") {
		t.Fatalf("expected smoke step message to mention warning, got %q", smokeStep.Message)
	}

	logs, total, err := service.ListStepLogs(context.Background(), &StepLogFilter{TaskID: task.ID, StepCode: StepCodeSmokeTest, Page: 1, PageSize: 200})
	if err != nil {
		t.Fatalf("ListStepLogs returned error: %v", err)
	}
	if total == 0 {
		t.Fatal("expected smoke test logs to be persisted")
	}
	foundWarn := false
	for _, logEntry := range logs {
		if logEntry.Level == LogLevelWarn && strings.Contains(logEntry.Message, "告警") {
			foundWarn = true
			break
		}
	}
	if !foundWarn {
		t.Fatalf("expected smoke test warning log, got %+v", logs)
	}
}

func TestService_SubscribeTaskEvents_receivesExecutionUpdates(t *testing.T) {
	database := openTestDB(t)
	repo := NewRepository(database)
	clusterOperator := &stubClusterOperator{}
	agentSender := &stubAgentCommandSender{agents: map[uint]string{101: "agent-node-a"}}
	service := newExecutionService(t, repo, clusterOperator, agentSender)

	planID := mustCreateReadyPlan(t, service)
	task, err := service.prepareExecutionTask(context.Background(), planID, 9)
	if err != nil {
		t.Fatalf("prepareExecutionTask returned error: %v", err)
	}

	events, cancel := service.SubscribeTaskEvents(task.ID)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, runErr := service.executeTask(context.Background(), task.ID)
		done <- runErr
	}()

	seenTaskUpdate := false
	seenStepUpdate := false
	seenNodeUpdate := false
	seenLogUpdate := false
	deadline := time.After(3 * time.Second)
	for !(seenTaskUpdate && seenStepUpdate && seenNodeUpdate && seenLogUpdate) {
		select {
		case event := <-events:
			switch event.EventType {
			case TaskEventTypeTaskUpdated:
				seenTaskUpdate = true
			case TaskEventTypeStepUpdated:
				seenStepUpdate = true
			case TaskEventTypeNodeUpdated:
				seenNodeUpdate = true
			case TaskEventTypeLogAppended:
				seenLogUpdate = true
			}
		case err := <-done:
			if err != nil {
				t.Fatalf("executeTask returned error: %v", err)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for task events: task=%t step=%t node=%t log=%t", seenTaskUpdate, seenStepUpdate, seenNodeUpdate, seenLogUpdate)
		}
	}
}

func newExecutionService(t *testing.T, repo *Repository, clusterOperator *stubClusterOperator, agentSender *stubAgentCommandSender) *Service {
	t.Helper()
	service := newPlanningService(t, repo)
	packagePath := createTestPackage(t, map[string]string{
		"config/seatunnel.yaml": "seatunnel: default",
	})
	service.SetPackageProvider(&stubPackageProvider{info: &installerapp.PackageInfo{
		Version:   "2.3.12",
		FileName:  "apache-seatunnel-2.3.12-bin.tar.gz",
		IsLocal:   true,
		LocalPath: packagePath,
		FileSize:  4096,
		Checksum:  "abc123",
	}})
	service.SetClusterOperator(clusterOperator)
	service.SetPackageTransferer(&stubPackageTransferer{})
	service.SetAgentCommandSender(agentSender)
	return service
}

func mustCreateReadyPlan(t *testing.T, service *Service) uint {
	t.Helper()
	result, err := service.CreatePlanFromRequest(context.Background(), &CreatePlanRequest{
		PrecheckRequest: PrecheckRequest{
			ClusterID:     1,
			TargetVersion: "2.3.12",
		},
	}, 7)
	if err != nil {
		t.Fatalf("CreatePlanFromRequest returned error: %v", err)
	}
	if result == nil || result.Plan == nil {
		t.Fatalf("expected ready plan to be created")
	}
	return result.Plan.ID
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func lastString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func hasSubCommand(commands []agentCommandRecord, subCommand string) bool {
	for _, command := range commands {
		if command.params["sub_command"] == subCommand {
			return true
		}
	}
	return false
}

func copiedHostID(agentID string, agents map[uint]string) uint {
	for hostID, currentAgentID := range agents {
		if currentAgentID == agentID {
			return hostID
		}
	}
	return 0
}
