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

package sync

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/db"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type stubConfigToolClient struct {
	webuiResp    *ConfigToolWebUIDAGResponse
	webuiErr     error
	dagResp      *ConfigToolDAGResponse
	dagErr       error
	validateResp *ConfigToolValidateResponse
	validateErr  error
	lastDAGReq   *ConfigToolContentRequest
	lastWebUIReq *ConfigToolContentRequest
	lastValidReq *ConfigToolValidateRequest
}

func (s *stubConfigToolClient) InspectDAG(ctx context.Context, endpoint string, req *ConfigToolContentRequest) (*ConfigToolDAGResponse, error) {
	s.lastDAGReq = req
	return s.dagResp, s.dagErr
}

func (s *stubConfigToolClient) InspectWebUIDAG(ctx context.Context, endpoint string, req *ConfigToolContentRequest) (*ConfigToolWebUIDAGResponse, error) {
	s.lastWebUIReq = req
	return s.webuiResp, s.webuiErr
}

func (s *stubConfigToolClient) ValidateConfig(ctx context.Context, endpoint string, req *ConfigToolValidateRequest) (*ConfigToolValidateResponse, error) {
	s.lastValidReq = req
	return s.validateResp, s.validateErr
}

func (s *stubConfigToolClient) DeriveSourcePreview(ctx context.Context, endpoint string, req *ConfigToolPreviewRequest) (*ConfigToolPreviewResponse, error) {
	return nil, nil
}

func (s *stubConfigToolClient) DeriveTransformPreview(ctx context.Context, endpoint string, req *ConfigToolPreviewRequest) (*ConfigToolPreviewResponse, error) {
	return nil, nil
}

func (s *stubConfigToolClient) ListPlugins(ctx context.Context, endpoint string, req *ConfigToolPluginListRequest) (*ConfigToolPluginListResponse, error) {
	return nil, nil
}

func (s *stubConfigToolClient) GetPluginOptions(ctx context.Context, endpoint string, req *ConfigToolPluginOptionsRequest) (*ConfigToolPluginOptionsResponse, error) {
	return nil, nil
}

func (s *stubConfigToolClient) RenderPluginTemplate(ctx context.Context, endpoint string, req *ConfigToolPluginTemplateRequest) (*ConfigToolPluginTemplateResponse, error) {
	return nil, nil
}

func (s *stubConfigToolClient) ListPluginEnumValues(ctx context.Context, endpoint string, req *ConfigToolPluginEnumValuesRequest) (*ConfigToolPluginEnumValuesResponse, error) {
	return nil, nil
}

func (s *stubConfigToolClient) PreviewSinkSaveMode(ctx context.Context, endpoint string, req *ConfigToolSinkSaveModePreviewRequest) (*ConfigToolSinkSaveModePreviewResponse, error) {
	return nil, nil
}

type stubConfigToolResolver struct {
	endpoint string
	err      error
}

func (s *stubConfigToolResolver) ResolveConfigToolEndpoint(ctx context.Context, clusterID uint, taskDefinition JSONMap) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.endpoint, nil
}

type stubAgentSender struct {
	success bool
	output  string
	err     error
}

func (s *stubAgentSender) SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error) {
	return s.success, s.output, s.err
}

type stubExecutionTargetResolver struct {
	targets []*ExecutionTarget
	err     error
}

func (s *stubExecutionTargetResolver) ResolveExecutionTarget(ctx context.Context, clusterID uint, definition JSONMap) (*ExecutionTarget, error) {
	if s.err != nil {
		return nil, s.err
	}
	if len(s.targets) == 0 {
		return nil, ErrExecutionTargetUnavailable
	}
	return s.targets[0], nil
}

func (s *stubExecutionTargetResolver) ResolveExecutionTargets(ctx context.Context, clusterID uint, definition JSONMap) ([]*ExecutionTarget, error) {
	if s.err != nil {
		return nil, s.err
	}
	if len(s.targets) == 0 {
		return nil, ErrExecutionTargetUnavailable
	}
	return s.targets, nil
}

func newTestSyncService(t *testing.T) *Service {
	t.Helper()

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&Task{}, &TaskVersion{}, &JobInstance{}, &GlobalVariable{}, &PreviewSession{}, &PreviewTable{}, &PreviewRow{}); err != nil {
		t.Fatalf("failed to migrate sync models: %v", err)
	}

	return NewService(NewRepository(database))
}

func uintPtr(value uint) *uint { return &value }

func TestCreateTaskRejectsUnsupportedName(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "root",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create folder failed: %v", err)
	}

	_, err = service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "bad name",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env {}",
		Definition:    JSONMap{},
	}, 1)
	if !errors.Is(err, ErrTaskNameInvalid) {
		t.Fatalf("expected ErrTaskNameInvalid, got %v", err)
	}
}

func TestUpdateTaskRejectsMovingFolderIntoDescendant(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()

	root, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "root",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create root folder failed: %v", err)
	}
	child, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(root.ID),
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "child",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create child folder failed: %v", err)
	}

	_, err = service.UpdateTask(ctx, root.ID, &UpdateTaskRequest{
		ParentID:      uintPtr(child.ID),
		NodeType:      string(TaskNodeTypeFolder),
		Name:          root.Name,
		ContentFormat: string(ContentFormatHOCON),
	})
	if !errors.Is(err, ErrTaskParentCycle) {
		t.Fatalf("expected ErrTaskParentCycle, got %v", err)
	}
}

func TestUpdateTaskAllowsMovingFileToFolder(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()

	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "folder_a",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create folder failed: %v", err)
	}
	sourceFolder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "folder_b",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create source folder failed: %v", err)
	}
	file, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(sourceFolder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "job_1",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env {}",
		Definition:    JSONMap{},
	}, 1)
	if err != nil {
		t.Fatalf("create file failed: %v", err)
	}

	updated, err := service.UpdateTask(ctx, file.ID, &UpdateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          file.Name,
		ContentFormat: string(ContentFormatHOCON),
		Content:       file.Content.String(),
		Definition:    file.Definition,
	})
	if err != nil {
		t.Fatalf("move file failed: %v", err)
	}
	if updated.ParentID == nil || *updated.ParentID != folder.ID {
		t.Fatalf("expected parent_id=%d, got %+v", folder.ID, updated.ParentID)
	}
}

func TestCreateTaskRejectsRootFile(t *testing.T) {
	service := newTestSyncService(t)

	_, err := service.CreateTask(context.Background(), &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFile),
		Name:          "root_job",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env {}",
		Definition:    JSONMap{},
	}, 1)
	if !errors.Is(err, ErrRootFileNotAllowed) {
		t.Fatalf("expected ErrRootFileNotAllowed, got %v", err)
	}
}

func TestCreateTaskRejectsDuplicateNameInSameFolder(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()

	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "dup_root",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create folder failed: %v", err)
	}
	if _, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "same_name",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env {}",
		Definition:    JSONMap{},
	}, 1); err != nil {
		t.Fatalf("create first file failed: %v", err)
	}
	_, err = service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "same_name",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if !errors.Is(err, ErrTaskNameDuplicate) {
		t.Fatalf("expected ErrTaskNameDuplicate, got %v", err)
	}
}

func TestRecoverJobUsesHistoricalSubmittedScriptWhenDraftIsNil(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()

	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "recover_root",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create folder failed: %v", err)
	}

	task, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "recover_job",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env { job.mode = \"STREAMING\" }\nsource { FakeSource { plugin_output = \"fake\" } }\nsink { Console {} }",
		Definition: JSONMap{
			"execution_mode": "cluster",
		},
	}, 1)
	if err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	if _, _, err := service.PublishTask(ctx, task.ID, "initial", 1); err != nil {
		t.Fatalf("publish task failed: %v", err)
	}
	updated, err := service.UpdateTask(ctx, task.ID, &UpdateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          task.Name,
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env { job.mode = \"STREAMING\" }\nsource { FakeSource { plugin_output = \"new_fake\" } }\nsink { Console {} }",
		Definition:    task.Definition,
	})
	if err != nil {
		t.Fatalf("update task failed: %v", err)
	}
	if strings.Contains(updated.Content.String(), "plugin_output = \"fake\"") {
		t.Fatalf("expected task content to change before recover")
	}

	source := &JobInstance{
		TaskID:        task.ID,
		TaskVersion:   1,
		RunType:       RunTypeRun,
		PlatformJobID: "177467000000000001",
		EngineJobID:   "177467000000000001",
		Status:        JobStatusSuccess,
		SubmitSpec: JSONMap{
			"mode":              "cluster",
			"format":            "hocon",
			"submitted_format":  "hocon",
			"submitted_content": "env { job.mode = \"STREAMING\" }\nsource { FakeSource { plugin_output = \"historical_fake\" } }\nsink { Console {} }",
			"job_name":          "historical_job",
			"platform_job_id":   "177467000000000001",
		},
		CreatedBy: 1,
	}
	if err := service.repo.CreateJobInstance(ctx, source); err != nil {
		t.Fatalf("create source job failed: %v", err)
	}

	recovered, err := service.RecoverJob(ctx, source.ID, 2, nil)
	if err != nil {
		t.Fatalf("recover job failed: %v", err)
	}
	if recovered.RunType != RunTypeRecover {
		t.Fatalf("expected recover run type, got %s", recovered.RunType)
	}
	if recovered.RecoveredFromInstanceID == nil || *recovered.RecoveredFromInstanceID != source.ID {
		t.Fatalf("expected recovered_from=%d, got %+v", source.ID, recovered.RecoveredFromInstanceID)
	}
	submitted := strings.TrimSpace(stringValue(recovered.SubmitSpec, "submitted_content"))
	if !strings.Contains(submitted, "historical_fake") {
		t.Fatalf("expected historical submitted content, got %q", submitted)
	}
	if strings.Contains(submitted, "new_fake") {
		t.Fatalf("expected recover to avoid current task draft/content, got %q", submitted)
	}
}

func TestUpdateTaskRejectsDuplicateSiblingName(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()

	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "rename_root",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create folder failed: %v", err)
	}
	left, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "left_job",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env {}",
		Definition:    JSONMap{},
	}, 1)
	if err != nil {
		t.Fatalf("create left file failed: %v", err)
	}
	right, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "right_job",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env {}",
		Definition:    JSONMap{},
	}, 1)
	if err != nil {
		t.Fatalf("create right file failed: %v", err)
	}
	_, err = service.UpdateTask(ctx, right.ID, &UpdateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          left.Name,
		ContentFormat: string(ContentFormatHOCON),
		Content:       right.Content.String(),
		Definition:    right.Definition,
	})
	if !errors.Is(err, ErrTaskNameDuplicate) {
		t.Fatalf("expected ErrTaskNameDuplicate, got %v", err)
	}
}

func TestDetectTemplateVariablesUsesPlatformSyntaxOnly(t *testing.T) {
	vars := detectTemplateVariables("{{ current_env }} ${seatunnel.builtin} {{job.name}}")
	if len(vars) != 2 {
		t.Fatalf("expected 2 variables, got %v", vars)
	}
	if vars[0] != "current_env" || vars[1] != "job.name" {
		t.Fatalf("unexpected variables: %v", vars)
	}
}

func TestDeleteTaskRemovesDescendantsAndRuntimeArtifacts(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()

	root, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "root_delete",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create root failed: %v", err)
	}
	file, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(root.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "job_delete",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env {}",
		Definition:    JSONMap{},
	}, 1)
	if err != nil {
		t.Fatalf("create file failed: %v", err)
	}
	if _, _, err := service.PublishTask(ctx, file.ID, "test", 1); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if err := service.repo.CreateJobInstance(ctx, &JobInstance{TaskID: file.ID, TaskVersion: 1, RunType: RunTypeRun, Status: JobStatusSuccess}); err != nil {
		t.Fatalf("create job instance failed: %v", err)
	}

	if err := service.DeleteTask(ctx, root.ID); err != nil {
		t.Fatalf("delete task failed: %v", err)
	}

	if _, err := service.repo.GetTaskByID(ctx, root.ID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected root deleted, got %v", err)
	}
	if _, err := service.repo.GetTaskByID(ctx, file.ID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("expected child file deleted, got %v", err)
	}
	jobs, total, err := service.repo.ListJobInstances(ctx, &JobFilter{TaskID: file.ID, Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("list jobs failed: %v", err)
	}
	if total != 0 || len(jobs) != 0 {
		t.Fatalf("expected job instances deleted, total=%d jobs=%d", total, len(jobs))
	}
}

func TestBuildTaskDAGPrefersWebUICompatibleDag(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()

	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "workspace",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create folder failed: %v", err)
	}
	file, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "demo_job",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env {}",
		Definition:    JSONMap{},
		ClusterID:     11,
	}, 1)
	if err != nil {
		t.Fatalf("create file failed: %v", err)
	}

	service.SetConfigToolClient(&stubConfigToolClient{
		webuiResp: &ConfigToolWebUIDAGResponse{
			JobID:     "preview",
			JobName:   "Config Preview",
			JobStatus: "CREATED",
			JobDag: ConfigToolWebUIJobDAG{
				JobID: "preview",
				PipelineEdges: map[string][]ConfigToolWebUIDAGEdge{
					"0": []ConfigToolWebUIDAGEdge{{InputVertexID: 1, TargetVertexID: 2}},
				},
				VertexInfoMap: map[string]ConfigToolWebUIDAGVertexInfo{
					"1": {VertexID: 1, Type: "source", ConnectorType: "Source[0]-FakeSource", TablePaths: []string{"fake"}},
					"2": {VertexID: 2, Type: "sink", ConnectorType: "Sink[0]-Console", TablePaths: []string{"fake"}},
				},
			},
			Metrics:     map[string]interface{}{"SourceReceivedCount": "0"},
			Warnings:    []string{"preview warning"},
			SimpleGraph: true,
		},
	})
	service.SetConfigToolResolver(&stubConfigToolResolver{endpoint: "http://127.0.0.1:18080"})

	result, err := service.BuildTaskDAG(ctx, file.ID, nil)
	if err != nil {
		t.Fatalf("BuildTaskDAG returned error: %v", err)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(result.Edges))
	}
	if result.WebUIJob == nil {
		t.Fatal("expected webui_job to be populated")
	}
	if result.WebUIJob["jobName"] != "Config Preview" {
		t.Fatalf("unexpected jobName: %#v", result.WebUIJob["jobName"])
	}
	if !result.SimpleGraph {
		t.Fatal("expected simple_graph to be true")
	}
	if len(result.Warnings) != 1 || result.Warnings[0] != "preview warning" {
		t.Fatalf("unexpected warnings: %#v", result.Warnings)
	}
}

func TestValidateTaskUsesConfigToolValidation(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "workspace",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create folder failed: %v", err)
	}
	file, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "demo_job",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env {}",
		Definition:    JSONMap{},
		ClusterID:     11,
	}, 1)
	if err != nil {
		t.Fatalf("create file failed: %v", err)
	}

	service.SetConfigToolClient(&stubConfigToolClient{
		validateResp: &ConfigToolValidateResponse{
			OK:      true,
			Valid:   true,
			Summary: "Config validation finished.",
			Warnings: []string{
				"connector warning",
			},
			Checks: []ConfigToolValidationCheck{{
				NodeID:        "source-0",
				Kind:          "source",
				ConnectorType: "Source[0]-Jdbc",
				Target:        "jdbc:mysql://127.0.0.1:3307/seatunnel_demo",
				Status:        "success",
				Message:       "Connection succeeded.",
			}},
		},
	})
	service.SetConfigToolResolver(&stubConfigToolResolver{endpoint: "http://127.0.0.1:18080"})

	result, err := service.ValidateTask(ctx, file.ID, nil)
	if err != nil {
		t.Fatalf("ValidateTask returned error: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected valid result, got %#v", result.Errors)
	}
	if len(result.Checks) != 1 || result.Checks[0].ConnectorType != "Source[0]-Jdbc" {
		t.Fatalf("unexpected checks: %#v", result.Checks)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warnings from config tool validation")
	}
}

func TestTestTaskConnectionsReturnsConfigToolChecks(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "workspace",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create folder failed: %v", err)
	}
	file, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "demo_job",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env {}",
		Definition:    JSONMap{},
		ClusterID:     11,
	}, 1)
	if err != nil {
		t.Fatalf("create file failed: %v", err)
	}

	service.SetConfigToolClient(&stubConfigToolClient{
		validateResp: &ConfigToolValidateResponse{
			OK:      false,
			Valid:   false,
			Summary: "Connection test finished.",
			Errors:  []string{"Sink[0]-Jdbc 连接失败: Access denied"},
			Checks: []ConfigToolValidationCheck{{
				NodeID:        "sink-0",
				Kind:          "sink",
				ConnectorType: "Sink[0]-Jdbc",
				Target:        "jdbc:mysql://127.0.0.1:3307/demo2",
				Status:        "failed",
				Message:       "Access denied",
			}},
		},
	})
	service.SetConfigToolResolver(&stubConfigToolResolver{endpoint: "http://127.0.0.1:18080"})

	result, err := service.TestTaskConnections(ctx, file.ID, nil)
	if err != nil {
		t.Fatalf("TestTaskConnections returned error: %v", err)
	}
	if result.Valid {
		t.Fatal("expected invalid result")
	}
	if len(result.Errors) != 1 {
		t.Fatalf("unexpected errors: %#v", result.Errors)
	}
	if len(result.Checks) != 1 || result.Checks[0].Status != "failed" {
		t.Fatalf("unexpected checks: %#v", result.Checks)
	}
}

func TestResolveTaskContentAppliesGlobalAndCustomVariables(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	if _, err := service.CreateGlobalVariable(ctx, &CreateGlobalVariableRequest{
		Key:   "global_env",
		Value: "prod",
	}, 1); err != nil {
		t.Fatalf("create global variable failed: %v", err)
	}

	task := &Task{
		Name:          "demo",
		ContentFormat: ContentFormatHOCON,
		Content:       "env = {{global_env}}\nsource = {{custom_name}}\nkeep = ${seatunnel.native}",
		Definition: JSONMap{
			"custom_variables": map[string]interface{}{
				"custom_name": "orders",
			},
		},
	}
	resolved, err := service.resolveTaskContent(ctx, task, &taskVariableRuntime{
		ReferenceTime: time.Date(2026, time.March, 28, 12, 30, 45, 0, time.Local),
	})
	if err != nil {
		t.Fatalf("resolve task content failed: %v", err)
	}
	if resolved != "env = prod\nsource = orders\nkeep = ${seatunnel.native}" {
		t.Fatalf("unexpected resolved content: %q", resolved)
	}
}

func TestResolveTaskContentPrefersCustomVariablesAndKeepsComplexValues(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	if _, err := service.CreateGlobalVariable(ctx, &CreateGlobalVariableRequest{
		Key:   "jdbc_url",
		Value: "jdbc://mysql:3306/global",
	}, 1); err != nil {
		t.Fatalf("create global variable failed: %v", err)
	}
	if _, err := service.CreateGlobalVariable(ctx, &CreateGlobalVariableRequest{
		Key:   "query_text",
		Value: `select * from "global.table"`,
	}, 1); err != nil {
		t.Fatalf("create global variable failed: %v", err)
	}

	task := &Task{
		Name:          "complex",
		ContentFormat: ContentFormatHOCON,
		Content:       "url = {{jdbc_url}}\nquery = {{query_text}}",
		Definition: JSONMap{
			"custom_variables": map[string]interface{}{
				"jdbc_url":   "jdbc://mysql:3306/test",
				"query_text": `select * from "aa.test"`,
			},
		},
	}
	resolved, err := service.resolveTaskContent(ctx, task, &taskVariableRuntime{
		ReferenceTime: time.Date(2026, time.March, 28, 12, 30, 45, 0, time.Local),
	})
	if err != nil {
		t.Fatalf("resolve task content failed: %v", err)
	}
	expected := "url = jdbc://mysql:3306/test\nquery = select * from \"aa.test\""
	if resolved != expected {
		t.Fatalf("unexpected resolved content: %q", resolved)
	}
}

func TestResolveTaskContentSupportsBuiltinTimeVariables(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()

	task := &Task{
		ID:            23,
		Name:          "time-demo",
		ContentFormat: ContentFormatHOCON,
		Content: db.ScriptText(strings.Join([]string{
			"biz_date = {{system.biz.date}}",
			"biz_curdate = {{system.biz.curdate}}",
			"datetime = {{system.datetime}}",
			"dt = {{yyyyMMdd-1}}",
			"month_start = {{month_first_day(yyyy-MM-dd,0)}}",
			"week_end = {{week_last_day(yyyyMMdd,0)}}",
			"native = ${table_name}",
		}, "\n")),
	}

	resolved, err := service.resolveTaskContent(ctx, task, &taskVariableRuntime{
		ReferenceTime: time.Date(2026, time.March, 28, 9, 8, 7, 0, time.Local),
		PlatformJobID: "1770000000001",
	})
	if err != nil {
		t.Fatalf("resolve task content failed: %v", err)
	}

	expected := strings.Join([]string{
		"biz_date = 20260327",
		"biz_curdate = 20260328",
		"datetime = 20260328090807",
		"dt = 20260327",
		"month_start = 2026-03-01",
		"week_end = 20260329",
		"native = ${table_name}",
	}, "\n")
	if resolved != expected {
		t.Fatalf("unexpected resolved content:\n%s", resolved)
	}
}

func TestCreateGlobalVariableRejectsReservedBuiltinVariableKey(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()

	_, err := service.CreateGlobalVariable(ctx, &CreateGlobalVariableRequest{
		Key:   "system.biz.date",
		Value: "20260328",
	}, 1)
	if !errors.Is(err, ErrReservedBuiltinVariableKey) {
		t.Fatalf("expected reserved builtin key error, got %v", err)
	}
}

func TestGlobalVariableSecretMaskingAndResolution(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()

	// 1. 创建保密凭据类型变量，验证返回脱敏为 ******
	// 1. Create secret variable, verify response is masked as ******
	created, err := service.CreateGlobalVariable(ctx, &CreateGlobalVariableRequest{
		Key:       "db_password",
		Value:     "SuperSecret123!",
		ValueType: GlobalVariableTypeSecret,
	}, 1)
	if err != nil {
		t.Fatalf("create secret global variable failed: %v", err)
	}
	if created.Value != "******" {
		t.Fatalf("expected created value to be masked as ******, got %q", created.Value)
	}
	if created.ValueType != GlobalVariableTypeSecret {
		t.Fatalf("expected created value_type to be secret, got %q", created.ValueType)
	}

	// 2. 分页查询，验证返回脱敏
	// 2. List paginated, verify returned value is masked
	items, total, err := service.ListGlobalVariablesPaginated(ctx, 1, 10)
	if err != nil {
		t.Fatalf("list global variables paginated failed: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected 1 item, got total=%d, len=%d", total, len(items))
	}
	if items[0].Value != "******" {
		t.Fatalf("expected listed value to be masked as ******, got %q", items[0].Value)
	}

	// 3. 验证任务解析运行时获取到真实密码（而非 ******）
	// 3. Verify task runtime variable resolution gets the real password (not ******)
	task := &Task{
		Name:          "secret-task",
		ContentFormat: ContentFormatHOCON,
		Content:       "password = \"{{db_password}}\"",
	}
	resolved, err := service.resolveTaskContent(ctx, task, &taskVariableRuntime{
		ReferenceTime: time.Now(),
	})
	if err != nil {
		t.Fatalf("resolve task content failed: %v", err)
	}
	if !strings.Contains(resolved, `password = "SuperSecret123!"`) {
		t.Fatalf("expected task content to contain real password, got:\n%s", resolved)
	}

	// 4. 更新变量时传空或掩码，验证数据库中原密码不被覆盖
	// 4. Update variable with empty or mask, verify original password in DB is retained
	updated, err := service.UpdateGlobalVariable(ctx, created.ID, &UpdateGlobalVariableRequest{
		Key:         "db_password",
		Value:       "", // 空值保留原密码 / Empty keeps existing secret
		ValueType:   GlobalVariableTypeSecret,
		Description: "updated description",
	})
	if err != nil {
		t.Fatalf("update global variable failed: %v", err)
	}
	if updated.Value != "******" {
		t.Fatalf("expected updated value to be masked as ******, got %q", updated.Value)
	}

	// 再次验证任务解析，依然是原真实密码
	// Verify task resolution again, should still be the original real password
	resolvedAfterEmptyUpdate, err := service.resolveTaskContent(ctx, task, &taskVariableRuntime{
		ReferenceTime: time.Now(),
	})
	if err != nil {
		t.Fatalf("resolve task content failed: %v", err)
	}
	if !strings.Contains(resolvedAfterEmptyUpdate, `password = "SuperSecret123!"`) {
		t.Fatalf("expected retained original password, got:\n%s", resolvedAfterEmptyUpdate)
	}

	// 5. 传新密码，验证密码成功更新
	// 5. Provide new password, verify password is successfully updated
	_, err = service.UpdateGlobalVariable(ctx, created.ID, &UpdateGlobalVariableRequest{
		Key:       "db_password",
		Value:     "BrandNewPassword456#",
		ValueType: GlobalVariableTypeSecret,
	})
	if err != nil {
		t.Fatalf("update new password failed: %v", err)
	}
	resolvedAfterNewPass, err := service.resolveTaskContent(ctx, task, &taskVariableRuntime{
		ReferenceTime: time.Now(),
	})
	if err != nil {
		t.Fatalf("resolve task content failed: %v", err)
	}
	if !strings.Contains(resolvedAfterNewPass, `password = "BrandNewPassword456#"`) {
		t.Fatalf("expected new password in task content, got:\n%s", resolvedAfterNewPass)
	}
}

func TestGlobalVariablePermissions(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()

	user1Actor := executionapp.Actor{UserID: 101, IsAdmin: false}
	user2Actor := executionapp.Actor{UserID: 102, IsAdmin: false}
	adminActor := executionapp.Actor{UserID: 1, IsAdmin: true}

	// 1. 普通用户 101 创建变量
	created, err := service.CreateGlobalVariable(ctx, &CreateGlobalVariableRequest{
		Key:         "user101_var",
		Value:       "val101",
		ValueType:   GlobalVariableTypeString,
		Description: "created by user 101",
	}, 101)
	if err != nil {
		t.Fatalf("user 101 create global variable failed: %v", err)
	}

	// 2. 普通用户 102 尝试修改用户 101 的变量，应拒绝 ErrGlobalVariablePermissionDenied
	_, err = service.UpdateGlobalVariableForActor(ctx, user2Actor, created.ID, &UpdateGlobalVariableRequest{
		Key:   "user101_var",
		Value: "hacked",
	})
	if !errors.Is(err, ErrGlobalVariablePermissionDenied) {
		t.Fatalf("expected ErrGlobalVariablePermissionDenied for other user, got: %v", err)
	}

	// 3. 普通用户 102 尝试删除用户 101 的变量，应拒绝 ErrGlobalVariablePermissionDenied
	err = service.DeleteGlobalVariableForActor(ctx, user2Actor, created.ID)
	if !errors.Is(err, ErrGlobalVariablePermissionDenied) {
		t.Fatalf("expected ErrGlobalVariablePermissionDenied for delete by other user, got: %v", err)
	}

	// 4. 创建者 101 修改自己的变量，应成功
	updated, err := service.UpdateGlobalVariableForActor(ctx, user1Actor, created.ID, &UpdateGlobalVariableRequest{
		Key:   "user101_var",
		Value: "val101_updated",
	})
	if err != nil {
		t.Fatalf("creator 101 update own variable failed: %v", err)
	}
	if updated.Value != "val101_updated" {
		t.Fatalf("expected updated value val101_updated, got %s", updated.Value)
	}

	// 5. 管理员可以修改任何人的变量
	adminUpdated, err := service.UpdateGlobalVariableForActor(ctx, adminActor, created.ID, &UpdateGlobalVariableRequest{
		Key:   "user101_var",
		Value: "val101_admin_override",
	})
	if err != nil {
		t.Fatalf("admin update variable failed: %v", err)
	}
	if adminUpdated.Value != "val101_admin_override" {
		t.Fatalf("expected admin override value, got %s", adminUpdated.Value)
	}

	// 6. 管理员删除变量
	if err := service.DeleteGlobalVariableForActor(ctx, adminActor, created.ID); err != nil {
		t.Fatalf("admin delete variable failed: %v", err)
	}
}

func TestCreateTaskRejectsReservedCustomVariableKey(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()

	root, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType: string(TaskNodeTypeFolder),
		Name:     "workspace",
	}, 1)
	if err != nil {
		t.Fatalf("create workspace root failed: %v", err)
	}

	_, err = service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFile),
		ParentID:      uintPtr(root.ID),
		Name:          "demo.hocon",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env { dt = {{system.biz.date}} }",
		Definition: JSONMap{
			"custom_variables": map[string]interface{}{
				"system.biz.date": "override",
			},
		},
	}, 1)
	if !errors.Is(err, ErrReservedBuiltinVariableKey) {
		t.Fatalf("expected reserved builtin key error, got %v", err)
	}
}

func TestValidateTaskUsesDraftContentWithoutSavingTask(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()

	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType: string(TaskNodeTypeFolder),
		Name:     "workspace",
	}, 1)
	if err != nil {
		t.Fatalf("create folder failed: %v", err)
	}
	file, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "draft-demo.hocon",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env { dt = \"saved\" }",
		Definition: JSONMap{
			"preview_http_sink": map[string]interface{}{"url": "http://127.0.0.1/collect"},
		},
	}, 1)
	if err != nil {
		t.Fatalf("create file failed: %v", err)
	}

	client := &stubConfigToolClient{
		validateResp: &ConfigToolValidateResponse{Valid: true, Summary: "ok"},
	}
	service.SetConfigToolClient(client)
	service.SetConfigToolResolver(&stubConfigToolResolver{endpoint: "http://127.0.0.1:18080"})

	_, err = service.ValidateTask(ctx, file.ID, &TaskDraftPayload{
		Name:          file.Name,
		Description:   file.Description,
		ClusterID:     file.ClusterID,
		EngineVersion: file.EngineVersion,
		Mode:          string(file.Mode),
		ContentFormat: string(file.ContentFormat),
		Content:       "env { dt = \"{{system.biz.curdate}}\" }",
		JobName:       file.JobName,
		Definition:    cloneJSONMap(file.Definition),
	})
	if err != nil {
		t.Fatalf("validate task failed: %v", err)
	}
	if client.lastValidReq == nil || !strings.Contains(client.lastValidReq.Content, "env { dt = ") {
		t.Fatalf("expected validate request to include draft content, got %#v", client.lastValidReq)
	}
	fresh, err := service.GetTask(ctx, file.ID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if fresh.Content != "env { dt = \"saved\" }" {
		t.Fatalf("expected stored content to remain unchanged, got %q", fresh.Content)
	}
}

func TestMergeLogChunksPreservesRepeatedLinesAcrossNodes(t *testing.T) {
	got := mergeLogChunks([]string{
		"2026-03-27 15:16:00 INFO start\n2026-03-27 15:16:01 WARN retry",
		"2026-03-27 15:16:01 WARN retry\n2026-03-27 15:16:02 ERROR failed",
	})

	expected := "2026-03-27 15:16:00 INFO start\n2026-03-27 15:16:01 WARN retry\n2026-03-27 15:16:01 WARN retry\n2026-03-27 15:16:02 ERROR failed"
	if got != expected {
		t.Fatalf("unexpected merged logs:\n%s", got)
	}
}

func TestGetTaskTreeAutoMovesRootFilesIntoWorkspaceFolder(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()

	if err := service.repo.CreateTask(ctx, &Task{
		NodeType:      TaskNodeTypeFile,
		Name:          "legacy_root_job",
		ContentFormat: ContentFormatHOCON,
		Content:       "env {}",
		Status:        TaskStatusDraft,
	}); err != nil {
		t.Fatalf("seed root file failed: %v", err)
	}

	tree, err := service.GetTaskTree(ctx)
	if err != nil {
		t.Fatalf("get task tree failed: %v", err)
	}
	if len(tree) != 1 {
		t.Fatalf("expected exactly one root folder after normalization, got %d", len(tree))
	}
	if tree[0].NodeType != TaskNodeTypeFolder {
		t.Fatalf("expected root node to be folder, got %s", tree[0].NodeType)
	}
	if len(tree[0].Children) != 1 || tree[0].Children[0].Name != "legacy_root_job" {
		t.Fatalf("expected root file moved under workspace folder, got %+v", tree[0].Children)
	}
}

func TestGetJobLogsReturnsEmptyPayloadWhenLevelFilterHasNoMatches(t *testing.T) {
	service := newTestSyncService(t)
	service.SetAgentCommandSender(&stubAgentSender{
		success: true,
		output:  `{"success":true,"message":"{\"logs\":\"\",\"path\":\"/opt/seatunnel/logs/job-177.log\",\"next_offset\":\"128\",\"file_size\":128}"}`,
	})
	service.SetExecutionTargetResolver(&stubExecutionTargetResolver{
		targets: []*ExecutionTarget{{
			AgentID:    "agent-1",
			InstallDir: "/opt/seatunnel",
			HostID:     1,
		}},
	})
	ctx := context.Background()
	if err := service.repo.CreateJobInstance(ctx, &JobInstance{
		TaskID:        1,
		TaskVersion:   1,
		RunType:       RunTypeRun,
		Status:        JobStatusRunning,
		PlatformJobID: "177",
		EngineJobID:   "177",
		SubmitSpec:    JSONMap{"cluster_id": 11, "target_agent_id": "agent-1", "install_dir": "/opt/seatunnel"},
		ResultPreview: JSONMap{},
		ErrorMessage:  "",
		CreatedBy:     1,
	}); err != nil {
		t.Fatalf("create job instance failed: %v", err)
	}
	result, err := service.GetJobLogs(ctx, 1, "", 64*1024, "", "error")
	if err != nil {
		t.Fatalf("GetJobLogs returned error: %v", err)
	}
	if result == nil {
		t.Fatalf("expected logs result, got nil")
	}
	if result.Logs != "" {
		t.Fatalf("expected empty logs, got %q", result.Logs)
	}
}

func TestGetJobLogsTreatsLegacyAgentPayloadWithoutPathAsAvailable(t *testing.T) {
	service := newTestSyncService(t)
	service.SetAgentCommandSender(&stubAgentSender{
		success: true,
		output:  `{"success":true,"message":"{\"logs\":\"\",\"next_offset\":\"128\",\"file_size\":128}"}`,
	})
	service.SetExecutionTargetResolver(&stubExecutionTargetResolver{
		targets: []*ExecutionTarget{{
			AgentID:    "agent-1",
			InstallDir: "/opt/seatunnel",
			HostID:     1,
		}},
	})
	ctx := context.Background()
	if err := service.repo.CreateJobInstance(ctx, &JobInstance{
		TaskID:        1,
		TaskVersion:   1,
		RunType:       RunTypeRun,
		Status:        JobStatusRunning,
		PlatformJobID: "177",
		EngineJobID:   "177",
		SubmitSpec:    JSONMap{"cluster_id": 11, "target_agent_id": "agent-1", "install_dir": "/opt/seatunnel"},
		ResultPreview: JSONMap{},
		CreatedBy:     1,
	}); err != nil {
		t.Fatalf("create job instance failed: %v", err)
	}
	result, err := service.GetJobLogs(ctx, 1, "", 64*1024, "", "error")
	if err != nil {
		t.Fatalf("GetJobLogs returned error: %v", err)
	}
	if result == nil {
		t.Fatalf("expected logs result, got nil")
	}
	if result.NextOffset == "" {
		t.Fatalf("expected next offset to be present for legacy payload")
	}
}

func TestCollectPreviewAppendsRowsIntoPreviewSession(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	now := time.Now()
	instance := &JobInstance{
		TaskID:        7,
		TaskVersion:   1,
		RunType:       RunTypePreview,
		Status:        JobStatusRunning,
		PlatformJobID: "preview-1",
		EngineJobID:   "preview-1",
		SubmitSpec:    JSONMap{},
		ResultPreview: JSONMap{},
		StartedAt:     &now,
		CreatedBy:     1,
	}
	if err := service.repo.CreateJobInstance(ctx, instance); err != nil {
		t.Fatalf("create job instance failed: %v", err)
	}
	if err := service.repo.CreatePreviewSession(ctx, &PreviewSession{
		JobInstanceID: instance.ID,
		TaskID:        instance.TaskID,
		PlatformJobID: instance.PlatformJobID,
		EngineJobID:   instance.EngineJobID,
		RowLimit:      3,
		Status:        "collecting",
		StartedAt:     &now,
	}); err != nil {
		t.Fatalf("create preview session failed: %v", err)
	}

	if err := service.CollectPreview(ctx, &PreviewCollectRequest{
		PlatformJobID: "preview-1",
		Dataset:       "seatunnel_demo.users",
		Columns:       []interface{}{"id", "name"},
		Rows: []map[string]interface{}{
			{"id": 1, "name": "a"},
			{"id": 2, "name": "b"},
		},
		RowLimit: 3,
	}); err != nil {
		t.Fatalf("collect preview failed: %v", err)
	}

	snapshot, err := service.GetPreviewSnapshot(ctx, instance.ID, "seatunnel_demo.users")
	if err != nil {
		t.Fatalf("get preview snapshot failed: %v", err)
	}
	if snapshot.TotalRows != 2 {
		t.Fatalf("expected total rows 2, got %d", snapshot.TotalRows)
	}
	if snapshot.SelectedTable == nil || len(snapshot.SelectedTable.Rows) != 2 {
		t.Fatalf("expected two selected table rows, got %+v", snapshot.SelectedTable)
	}
}

func TestCollectPreviewStopsAtRowLimit(t *testing.T) {
	service := newTestSyncService(t)
	service.engineClient = &stubEngineClient{info: &EngineJobInfo{JobID: "preview-2", JobStatus: "CANCELED"}}
	ctx := context.Background()
	now := time.Now()
	instance := &JobInstance{
		TaskID:        8,
		TaskVersion:   1,
		RunType:       RunTypePreview,
		Status:        JobStatusRunning,
		PlatformJobID: "preview-2",
		EngineJobID:   "preview-2",
		SubmitSpec:    JSONMap{"engine_base_url": "http://127.0.0.1:8080"},
		ResultPreview: JSONMap{},
		StartedAt:     &now,
		CreatedBy:     1,
	}
	if err := service.repo.CreateJobInstance(ctx, instance); err != nil {
		t.Fatalf("create job instance failed: %v", err)
	}
	if err := service.repo.CreatePreviewSession(ctx, &PreviewSession{
		JobInstanceID: instance.ID,
		TaskID:        instance.TaskID,
		PlatformJobID: instance.PlatformJobID,
		EngineJobID:   instance.EngineJobID,
		RowLimit:      1,
		Status:        "collecting",
		StartedAt:     &now,
	}); err != nil {
		t.Fatalf("create preview session failed: %v", err)
	}

	if err := service.CollectPreview(ctx, &PreviewCollectRequest{
		PlatformJobID: "preview-2",
		Dataset:       "seatunnel_demo.users",
		Columns:       []interface{}{"id"},
		Rows: []map[string]interface{}{
			{"id": 1},
			{"id": 2},
		},
		RowLimit: 1,
	}); err != nil {
		t.Fatalf("collect preview failed: %v", err)
	}

	snapshot, err := service.GetPreviewSnapshot(ctx, instance.ID, "seatunnel_demo.users")
	if err != nil {
		t.Fatalf("get preview snapshot failed: %v", err)
	}
	if !snapshot.Truncated {
		t.Fatalf("expected preview snapshot to be truncated")
	}
	if snapshot.TotalRows != 1 {
		t.Fatalf("expected total rows 1, got %d", snapshot.TotalRows)
	}
	job, err := service.GetJob(ctx, instance.ID)
	if err != nil {
		t.Fatalf("get job failed: %v", err)
	}
	if job.Status != JobStatusCanceled {
		t.Fatalf("expected preview job canceled after reaching row limit, got %s", job.Status)
	}
}

func TestGetPreviewSnapshotReturnsEmptySnapshotWhenSessionNotReady(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	now := time.Now()
	instance := &JobInstance{
		TaskID:        9,
		TaskVersion:   1,
		RunType:       RunTypePreview,
		Status:        JobStatusRunning,
		PlatformJobID: "preview-empty",
		EngineJobID:   "preview-empty",
		SubmitSpec: JSONMap{
			"row_limit":       100,
			"timeout_minutes": 10,
		},
		ResultPreview: JSONMap{},
		StartedAt:     &now,
		CreatedBy:     1,
	}
	if err := service.repo.CreateJobInstance(ctx, instance); err != nil {
		t.Fatalf("create job instance failed: %v", err)
	}

	snapshot, err := service.GetPreviewSnapshot(ctx, instance.ID, "")
	if err != nil {
		t.Fatalf("get preview snapshot failed: %v", err)
	}
	if snapshot.EmptyReason != "preview_not_ready" {
		t.Fatalf("expected preview_not_ready, got %q", snapshot.EmptyReason)
	}
	if len(snapshot.Tables) != 0 {
		t.Fatalf("expected no preview tables, got %d", len(snapshot.Tables))
	}
}

func TestGetJobLogsReturnsEmptyResultWhenUnavailable(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	instance := &JobInstance{
		TaskID:        10,
		TaskVersion:   1,
		RunType:       RunTypeRun,
		Status:        JobStatusSuccess,
		PlatformJobID: "run-no-logs",
		EngineJobID:   "run-no-logs",
		SubmitSpec: JSONMap{
			"execution_mode": "cluster",
		},
		ResultPreview: JSONMap{},
		CreatedBy:     1,
	}
	if err := service.repo.CreateJobInstance(ctx, instance); err != nil {
		t.Fatalf("create job instance failed: %v", err)
	}

	result, err := service.GetJobLogs(ctx, instance.ID, "", 64*1024, "", "")
	if err != nil {
		t.Fatalf("get job logs failed: %v", err)
	}
	if result.EmptyReason != "logs_not_ready" {
		t.Fatalf("expected logs_not_ready, got %q", result.EmptyReason)
	}
	if result.Logs != "" {
		t.Fatalf("expected empty logs, got %q", result.Logs)
	}
}

func TestUpdateTaskRejectsInvalidSchedule(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "workspace",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create folder failed: %v", err)
	}
	_, err = service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "job_schedule_invalid",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env {}",
		Definition: JSONMap{
			"schedule": JSONMap{
				"enabled":   true,
				"cron_expr": "bad cron",
				"timezone":  "Asia/Shanghai",
			},
		},
	}, 1)
	if !errors.Is(err, ErrInvalidTaskSchedule) {
		t.Fatalf("expected ErrInvalidTaskSchedule, got %v", err)
	}
}

func TestListTasksDecoratesScheduleMetadata(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "workspace",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create folder failed: %v", err)
	}
	file, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "scheduled_meta_job",
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env {}",
		Definition: JSONMap{
			"schedule": JSONMap{
				"enabled":   true,
				"cron_expr": "0 9 * * *",
				"timezone":  "Asia/Shanghai",
			},
		},
	}, 1)
	if err != nil {
		t.Fatalf("create file failed: %v", err)
	}
	startedAt := time.Date(2026, 3, 29, 9, 0, 0, 0, time.UTC)
	if err := service.repo.CreateJobInstance(ctx, &JobInstance{
		TaskID:        file.ID,
		TaskVersion:   1,
		RunType:       RunTypeSchedule,
		PlatformJobID: "scheduled-1",
		Status:        JobStatusSuccess,
		StartedAt:     &startedAt,
		CreatedBy:     1,
	}); err != nil {
		t.Fatalf("create scheduled job failed: %v", err)
	}
	items, total, err := service.ListTasks(ctx, &TaskFilter{Page: 1, Size: 20})
	if err != nil {
		t.Fatalf("list tasks failed: %v", err)
	}
	if total == 0 || len(items) == 0 {
		t.Fatalf("expected listed tasks, got total=%d len=%d", total, len(items))
	}
	var got *Task
	for _, item := range items {
		if item != nil && item.ID == file.ID {
			got = item
			break
		}
	}
	if got == nil {
		t.Fatalf("expected scheduled task in list")
	}
	if !got.ScheduleEnabled {
		t.Fatalf("expected schedule enabled")
	}
	if got.ScheduleCronExpr != "0 9 * * *" {
		t.Fatalf("unexpected cron expr %q", got.ScheduleCronExpr)
	}
	if got.ScheduleTimezone != "Asia/Shanghai" {
		t.Fatalf("unexpected timezone %q", got.ScheduleTimezone)
	}
	if got.ScheduleLastTriggeredAt == nil || !got.ScheduleLastTriggeredAt.Equal(startedAt) {
		t.Fatalf("unexpected last triggered at %#v", got.ScheduleLastTriggeredAt)
	}
	if got.ScheduleNextTriggeredAt == nil {
		t.Fatalf("expected next triggered at")
	}
}

func TestSubmitScheduledTaskUsesPublishedVersionSnapshot(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType:      string(TaskNodeTypeFolder),
		Name:          "workspace",
		ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create folder failed: %v", err)
	}
	file, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      uintPtr(folder.ID),
		NodeType:      string(TaskNodeTypeFile),
		Name:          "scheduled_job",
		ClusterID:     11,
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env { job.mode = \"batch\" } source { FakeSource { plugin_output = \"fake\" row.num = 1 schema = { fields { name = \"string\" } } } } sink { Console {} }",
		Definition: JSONMap{
			"schedule": JSONMap{
				"enabled":   true,
				"cron_expr": "0 9 * * *",
				"timezone":  "Asia/Shanghai",
			},
		},
	}, 1)
	if err != nil {
		t.Fatalf("create file failed: %v", err)
	}
	_, version, err := service.PublishTask(ctx, file.ID, "initial", 1)
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	_, err = service.UpdateTask(ctx, file.ID, &UpdateTaskRequest{
		ParentID:      file.ParentID,
		NodeType:      string(TaskNodeTypeFile),
		Name:          file.Name,
		Description:   file.Description,
		ClusterID:     file.ClusterID,
		EngineVersion: file.EngineVersion,
		Mode:          string(file.Mode),
		ContentFormat: string(file.ContentFormat),
		Content:       "env { job.mode = \"batch\" } source { FakeSource { plugin_output = \"fake\" row.num = 999 schema = { fields { changed = \"string\" } } } } sink { Console {} }",
		JobName:       file.JobName,
		Definition:    file.Definition,
		SortOrder:     file.SortOrder,
	})
	if err != nil {
		t.Fatalf("update task failed: %v", err)
	}
	freshTask, err := service.repo.GetTaskByID(ctx, file.ID)
	if err != nil {
		t.Fatalf("reload task failed: %v", err)
	}
	if err := service.submitScheduledTask(ctx, freshTask); err != nil {
		t.Fatalf("submitScheduledTask failed: %v", err)
	}
	jobs, total, err := service.repo.ListJobInstances(ctx, &JobFilter{TaskID: file.ID, Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("list jobs failed: %v", err)
	}
	if total != 1 || len(jobs) != 1 {
		t.Fatalf("expected one scheduled job, got total=%d len=%d", total, len(jobs))
	}
	job := jobs[0]
	if job.RunType != RunTypeSchedule {
		t.Fatalf("expected run type schedule, got %s", job.RunType)
	}
	if got := stringValue(job.SubmitSpec, "trigger_source"); got != scheduleTriggerSource {
		t.Fatalf("expected trigger_source schedule, got %q", got)
	}
	if job.TaskVersion != version.Version {
		t.Fatalf("expected task version %d, got %d", version.Version, job.TaskVersion)
	}
	submitted := stringValue(job.SubmitSpec, "submitted_content")
	if !strings.Contains(submitted, "row.num = 1") {
		t.Fatalf("expected published snapshot content, got %s", submitted)
	}
	if strings.Contains(submitted, "row.num = 999") {
		t.Fatalf("unexpected draft content used for schedule: %s", submitted)
	}
}

type stubEngineClient struct {
	info      *EngineJobInfo
	stopErr   error
	stopCalls int
}

func (s *stubEngineClient) Submit(ctx context.Context, req *EngineSubmitRequest) (*EngineSubmitResponse, error) {
	return nil, nil
}
func (s *stubEngineClient) GetJobInfo(ctx context.Context, endpoint *EngineEndpoint, jobID string) (*EngineJobInfo, error) {
	return s.info, nil
}
func (s *stubEngineClient) GetJobCheckpointOverview(ctx context.Context, endpoint *EngineEndpoint, jobID string) (*EngineCheckpointOverview, error) {
	return nil, nil
}
func (s *stubEngineClient) GetJobCheckpointHistory(ctx context.Context, endpoint *EngineEndpoint, jobID string, pipelineID *int, limit int, status string) ([]*EngineCheckpointRecord, error) {
	return nil, nil
}
func (s *stubEngineClient) StopJob(ctx context.Context, endpoint *EngineEndpoint, jobID string, stopWithSavepoint bool) error {
	s.stopCalls++
	return s.stopErr
}
func (s *stubEngineClient) GetJobLogs(ctx context.Context, endpoint *EngineEndpoint, jobID string) (string, error) {
	return "", nil
}

func TestRefreshJobInstanceUsesEngineFinishedTime(t *testing.T) {
	service := newTestSyncService(t)
	service.engineClient = &stubEngineClient{info: &EngineJobInfo{
		JobID:        "engine-job-1",
		JobStatus:    "FINISHED",
		FinishedTime: "2026-03-29 15:50:44",
	}}
	ctx := context.Background()
	job := &JobInstance{
		TaskID:        1,
		TaskVersion:   1,
		RunType:       RunTypeSchedule,
		Status:        JobStatusRunning,
		PlatformJobID: "platform-1",
		EngineJobID:   "engine-job-1",
		SubmitSpec: JSONMap{
			"engine_base_url": "http://127.0.0.1:8080",
		},
		ResultPreview: JSONMap{},
		CreatedBy:     1,
	}
	if err := service.repo.CreateJobInstance(ctx, job); err != nil {
		t.Fatalf("create job instance failed: %v", err)
	}
	refreshed, err := service.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("refresh job failed: %v", err)
	}
	if refreshed.Status != JobStatusSuccess {
		t.Fatalf("expected success status, got %s", refreshed.Status)
	}
	if refreshed.FinishedAt == nil {
		t.Fatalf("expected finished_at to be populated")
	}
	if got := refreshed.FinishedAt.Format(time.DateTime); got != "2026-03-29 15:50:44" {
		t.Fatalf("expected engine finished time, got %s", got)
	}
}

func TestGetPreviewSnapshotKeepsTerminalStateWhenEngineReturnsStaleRunningStatus(t *testing.T) {
	service := newTestSyncService(t)
	if err := service.repo.db.AutoMigrate(&executionapp.Execution{}, &executionapp.Confirmation{}); err != nil {
		t.Fatalf("迁移公共执行表失败: %v", err)
	}
	executionService := executionapp.NewService(executionapp.NewRepository(service.repo.db), executionapp.NewProviderRegistry())
	service.SetExecutionService(executionService)
	service.engineClient = &stubEngineClient{info: &EngineJobInfo{JobID: "preview-terminal", JobStatus: "RUNNING"}}
	ctx := context.Background()
	execution, _, err := executionService.Create(ctx, executionapp.CreateInput{
		OperationID: "sync.task.preview",
		OwnerUserID: 1,
		ActorType:   executionapp.ActorTypeUser,
		Module:      syncExecutionModule,
		Status:      executionapp.StatusCancelled,
		Cancellable: false,
	})
	if err != nil {
		t.Fatalf("创建公共执行记录失败: %v", err)
	}
	finishedAt := time.Now()
	job := &JobInstance{
		TaskID:        1,
		TaskVersion:   1,
		RunType:       RunTypePreview,
		Status:        JobStatusCanceled,
		PlatformJobID: "preview-terminal",
		EngineJobID:   "preview-terminal",
		ExecutionID:   execution.ExecutionID,
		SubmitSpec:    JSONMap{"engine_base_url": "http://127.0.0.1:8080"},
		ResultPreview: JSONMap{"job_status": "CANCEL_REQUESTED"},
		FinishedAt:    &finishedAt,
		CreatedBy:     1,
	}
	if err := service.repo.CreateJobInstance(ctx, job); err != nil {
		t.Fatalf("创建预览作业失败: %v", err)
	}
	if err := executionService.BindModuleRef(ctx, execution.ExecutionID, strconv.FormatUint(uint64(job.ID), 10)); err != nil {
		t.Fatalf("绑定公共执行记录失败: %v", err)
	}

	snapshot, err := service.GetPreviewSnapshot(ctx, job.ID, "")
	if err != nil {
		t.Fatalf("读取预览快照失败: %v", err)
	}
	if snapshot.Status != string(JobStatusCanceled) {
		t.Fatalf("迟到的运行状态不应覆盖作业终态，实际为 %s", snapshot.Status)
	}
	storedExecution, err := executionService.Get(ctx, executionapp.Actor{UserID: 1}, execution.ExecutionID)
	if err != nil {
		t.Fatalf("读取公共执行记录失败: %v", err)
	}
	if storedExecution.Status != executionapp.StatusCancelled {
		t.Fatalf("迟到的运行状态不应覆盖公共执行终态，实际为 %s", storedExecution.Status)
	}
}

func TestGetPreviewSnapshotKeepsRunningStateWhenEngineReturnsCreated(t *testing.T) {
	service := newTestSyncService(t)
	if err := service.repo.db.AutoMigrate(&executionapp.Execution{}, &executionapp.Confirmation{}); err != nil {
		t.Fatalf("迁移公共执行表失败: %v", err)
	}
	executionService := executionapp.NewService(executionapp.NewRepository(service.repo.db), executionapp.NewProviderRegistry())
	service.SetExecutionService(executionService)
	service.engineClient = &stubEngineClient{info: &EngineJobInfo{JobID: "preview-created", JobStatus: "CREATED"}}
	ctx := context.Background()
	execution, _, err := executionService.Create(ctx, executionapp.CreateInput{
		OperationID: "sync.task.preview",
		OwnerUserID: 1,
		ActorType:   executionapp.ActorTypeUser,
		Module:      syncExecutionModule,
		Status:      executionapp.StatusRunning,
		Cancellable: true,
	})
	if err != nil {
		t.Fatalf("创建公共执行记录失败: %v", err)
	}
	job := &JobInstance{
		TaskID:        1,
		TaskVersion:   1,
		RunType:       RunTypePreview,
		Status:        JobStatusRunning,
		PlatformJobID: "preview-created",
		EngineJobID:   "preview-created",
		ExecutionID:   execution.ExecutionID,
		SubmitSpec:    JSONMap{"engine_base_url": "http://127.0.0.1:8080"},
		ResultPreview: JSONMap{},
		CreatedBy:     1,
	}
	if err := service.repo.CreateJobInstance(ctx, job); err != nil {
		t.Fatalf("创建预览作业失败: %v", err)
	}
	if err := executionService.BindModuleRef(ctx, execution.ExecutionID, strconv.FormatUint(uint64(job.ID), 10)); err != nil {
		t.Fatalf("绑定公共执行记录失败: %v", err)
	}

	snapshot, err := service.GetPreviewSnapshot(ctx, job.ID, "")
	if err != nil {
		t.Fatalf("CREATED 状态不应导致预览读取失败: %v", err)
	}
	if snapshot.Status != string(JobStatusRunning) {
		t.Fatalf("CREATED 状态不应把运行中任务改回 pending，实际为 %s", snapshot.Status)
	}
	storedExecution, err := executionService.Get(ctx, executionapp.Actor{UserID: 1}, execution.ExecutionID)
	if err != nil {
		t.Fatalf("读取公共执行记录失败: %v", err)
	}
	if storedExecution.Status != executionapp.StatusRunning {
		t.Fatalf("公共执行不应从 running 倒退，实际为 %s", storedExecution.Status)
	}
}

func TestSyncExecutionFromJobIgnoresStaleStatusAfterTerminalState(t *testing.T) {
	service := newTestSyncService(t)
	if err := service.repo.db.AutoMigrate(&executionapp.Execution{}, &executionapp.Confirmation{}); err != nil {
		t.Fatalf("迁移公共执行表失败: %v", err)
	}
	executionService := executionapp.NewService(executionapp.NewRepository(service.repo.db), executionapp.NewProviderRegistry())
	service.SetExecutionService(executionService)
	ctx := context.Background()
	execution, _, err := executionService.Create(ctx, executionapp.CreateInput{
		OperationID: "sync.task.preview",
		OwnerUserID: 1,
		ActorType:   executionapp.ActorTypeUser,
		Module:      syncExecutionModule,
		Status:      executionapp.StatusSucceeded,
	})
	if err != nil {
		t.Fatalf("创建公共执行记录失败: %v", err)
	}

	err = service.syncExecutionFromJob(ctx, &JobInstance{
		ID:          1,
		Status:      JobStatusRunning,
		ExecutionID: execution.ExecutionID,
		CreatedBy:   1,
	})
	if err != nil {
		t.Fatalf("迟到状态不应导致只读刷新失败: %v", err)
	}
	storedExecution, err := executionService.Get(ctx, executionapp.Actor{UserID: 1}, execution.ExecutionID)
	if err != nil {
		t.Fatalf("读取公共执行记录失败: %v", err)
	}
	if storedExecution.Status != executionapp.StatusSucceeded {
		t.Fatalf("公共执行终态不应被迟到状态覆盖，实际为 %s", storedExecution.Status)
	}
}

func TestSyncExecutionFromJobIgnoresStalePendingStatus(t *testing.T) {
	service := newTestSyncService(t)
	if err := service.repo.db.AutoMigrate(&executionapp.Execution{}, &executionapp.Confirmation{}); err != nil {
		t.Fatalf("迁移公共执行表失败: %v", err)
	}
	executionService := executionapp.NewService(executionapp.NewRepository(service.repo.db), executionapp.NewProviderRegistry())
	service.SetExecutionService(executionService)
	ctx := context.Background()
	execution, _, err := executionService.Create(ctx, executionapp.CreateInput{
		OperationID: "sync.task.preview",
		OwnerUserID: 1,
		ActorType:   executionapp.ActorTypeUser,
		Module:      syncExecutionModule,
		Status:      executionapp.StatusRunning,
	})
	if err != nil {
		t.Fatalf("创建公共执行记录失败: %v", err)
	}

	err = service.syncExecutionFromJob(ctx, &JobInstance{
		ID:          1,
		Status:      JobStatusPending,
		ExecutionID: execution.ExecutionID,
		CreatedBy:   1,
	})
	if err != nil {
		t.Fatalf("迟到的 pending 状态不应导致只读刷新失败: %v", err)
	}
	storedExecution, err := executionService.Get(ctx, executionapp.Actor{UserID: 1}, execution.ExecutionID)
	if err != nil {
		t.Fatalf("读取公共执行记录失败: %v", err)
	}
	if storedExecution.Status != executionapp.StatusRunning {
		t.Fatalf("公共执行不应从 running 倒退到 pending，实际为 %s", storedExecution.Status)
	}
}

func TestCancelJobWaitsForEngineConfirmation(t *testing.T) {
	service := newTestSyncService(t)
	engine := &stubEngineClient{info: &EngineJobInfo{JobID: "engine-cancel-1", JobStatus: "RUNNING"}}
	service.engineClient = engine
	ctx := context.Background()
	job := &JobInstance{
		TaskID:        1,
		TaskVersion:   1,
		RunType:       RunTypeRun,
		Status:        JobStatusRunning,
		PlatformJobID: "platform-cancel-1",
		EngineJobID:   "engine-cancel-1",
		SubmitSpec:    JSONMap{"engine_base_url": "http://127.0.0.1:8080"},
		ResultPreview: JSONMap{},
		CreatedBy:     1,
	}
	if err := service.repo.CreateJobInstance(ctx, job); err != nil {
		t.Fatalf("创建作业失败: %v", err)
	}

	cancelled, err := service.CancelJob(ctx, job.ID, false)
	if err != nil {
		t.Fatalf("请求取消失败: %v", err)
	}
	if cancelled.Status != JobStatusCancelling {
		t.Fatalf("停止请求成功后应处于 cancelling，实际为 %s", cancelled.Status)
	}
	if engine.stopCalls != 1 {
		t.Fatalf("停止接口调用次数错误: %d", engine.stopCalls)
	}
	if _, err := service.CancelJob(ctx, job.ID, false); err != nil {
		t.Fatalf("重复取消应幂等: %v", err)
	}
	if engine.stopCalls != 1 {
		t.Fatalf("重复取消不应再次调用停止接口: %d", engine.stopCalls)
	}

	refreshed, err := service.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("刷新取消中作业失败: %v", err)
	}
	if refreshed.Status != JobStatusCancelling {
		t.Fatalf("引擎仍运行时不能宣称已取消: %s", refreshed.Status)
	}

	engine.info = &EngineJobInfo{JobID: "engine-cancel-1", JobStatus: "CANCELED"}
	refreshed, err = service.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("读取引擎取消结果失败: %v", err)
	}
	if refreshed.Status != JobStatusCanceled {
		t.Fatalf("引擎确认后应进入 canceled，实际为 %s", refreshed.Status)
	}
}

func TestCancelJobFailureRestoresRunningStatus(t *testing.T) {
	service := newTestSyncService(t)
	service.engineClient = &stubEngineClient{stopErr: errors.New("stop unavailable")}
	ctx := context.Background()
	job := &JobInstance{
		TaskID:        1,
		TaskVersion:   1,
		RunType:       RunTypeRun,
		Status:        JobStatusRunning,
		PlatformJobID: "platform-cancel-2",
		EngineJobID:   "engine-cancel-2",
		SubmitSpec:    JSONMap{"engine_base_url": "http://127.0.0.1:8080"},
		ResultPreview: JSONMap{},
		CreatedBy:     1,
	}
	if err := service.repo.CreateJobInstance(ctx, job); err != nil {
		t.Fatalf("创建作业失败: %v", err)
	}
	if _, err := service.CancelJob(ctx, job.ID, false); err == nil {
		t.Fatal("停止接口失败时应返回错误")
	}
	stored, err := service.repo.GetJobInstanceByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("读取作业失败: %v", err)
	}
	if stored.Status != JobStatusRunning {
		t.Fatalf("停止失败后应恢复 running，实际为 %s", stored.Status)
	}
	if stored.ErrorMessage != "cancel request failed" {
		t.Fatalf("停止失败摘要错误: %q", stored.ErrorMessage)
	}
}

func TestJobOwnershipFiltersListAndDetail(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	for _, owner := range []uint{1, 2} {
		job := &JobInstance{
			TaskID:        owner,
			TaskVersion:   1,
			RunType:       RunTypeRun,
			Status:        JobStatusRunning,
			PlatformJobID: "platform-owner-" + strconv.FormatUint(uint64(owner), 10),
			CreatedBy:     owner,
		}
		if err := service.repo.CreateJobInstance(ctx, job); err != nil {
			t.Fatalf("创建用户 %d 的作业失败: %v", owner, err)
		}
	}

	items, total, err := service.ListJobsForActor(ctx, executionapp.Actor{UserID: 1}, &JobFilter{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("按用户列出作业失败: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].CreatedBy != 1 {
		t.Fatalf("普通用户看到了其他用户作业: total=%d items=%+v", total, items)
	}
	if _, err := service.GetJobForActor(ctx, executionapp.Actor{UserID: 1}, items[0].ID+1); !errors.Is(err, ErrJobInstanceNotFound) {
		t.Fatalf("普通用户读取他人作业应返回未找到: %v", err)
	}
	adminItems, adminTotal, err := service.ListJobsForActor(ctx, executionapp.Actor{UserID: 9, IsAdmin: true}, &JobFilter{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("管理员列出全部作业失败: %v", err)
	}
	if adminTotal != 2 || len(adminItems) != 2 {
		t.Fatalf("管理员应看到全部作业: total=%d len=%d", adminTotal, len(adminItems))
	}
}

func TestRunWithExecutionReusesIdempotentJob(t *testing.T) {
	service := newTestSyncService(t)
	if err := service.repo.db.AutoMigrate(&executionapp.Execution{}, &executionapp.Confirmation{}); err != nil {
		t.Fatalf("迁移公共执行表失败: %v", err)
	}
	executionRepo := executionapp.NewRepository(service.repo.db)
	executionService := executionapp.NewService(executionRepo, executionapp.NewProviderRegistry())
	service.SetExecutionService(executionService)
	ctx := context.Background()
	runCalls := 0
	run := func(runCtx context.Context) (*JobInstance, error) {
		runCalls++
		job := &JobInstance{
			TaskID:        1,
			TaskVersion:   1,
			RunType:       RunTypeRun,
			Status:        JobStatusRunning,
			PlatformJobID: "platform-idempotent",
			CreatedBy:     1,
		}
		if err := service.repo.CreateJobInstance(runCtx, job); err != nil {
			return nil, err
		}
		return job, nil
	}
	request := ExecutionRequest{RequestID: "request-1", IdempotencyKey: "same-key", RequestHash: "same-request"}
	first, err := service.runWithExecution(ctx, 1, "sync.task.submit", executionapp.RiskLevelR1, request, run)
	if err != nil {
		t.Fatalf("首次执行失败: %v", err)
	}
	second, err := service.runWithExecution(ctx, 1, "sync.task.submit", executionapp.RiskLevelR1, request, run)
	if err != nil {
		t.Fatalf("幂等重试失败: %v", err)
	}
	if runCalls != 1 {
		t.Fatalf("同一幂等键启动了多次实际任务: %d", runCalls)
	}
	if first.ID != second.ID || first.ExecutionID == "" || first.ExecutionID != second.ExecutionID {
		t.Fatalf("幂等重试没有返回同一作业: first=%+v second=%+v", first, second)
	}
	item, err := executionService.Get(ctx, executionapp.Actor{UserID: 1}, first.ExecutionID)
	if err != nil {
		t.Fatalf("读取公共执行记录失败: %v", err)
	}
	if item.Status != executionapp.StatusRunning || item.ModuleRef != strconv.FormatUint(uint64(first.ID), 10) {
		t.Fatalf("公共执行记录错误: %+v", item)
	}
}

func TestEngineJobInfoUsesFinishTimeField(t *testing.T) {
	var info EngineJobInfo
	payload := []byte(`{"jobId":"1","jobStatus":"FINISHED","finishTime":"2026-03-29 16:18:59"}`)
	if err := json.Unmarshal(payload, &info); err != nil {
		t.Fatalf("unmarshal engine job info failed: %v", err)
	}
	if info.FinishedTime != "2026-03-29 16:18:59" {
		t.Fatalf("expected finishTime to populate FinishedTime, got %q", info.FinishedTime)
	}
}

func TestTaskPermissionsAndRoles(t *testing.T) {
	ctx := context.Background()
	service := newTestSyncService(t)

	// 1. Create a parent folder and a public task created by admin (userID 1)
	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType: string(TaskNodeTypeFolder),
		Name:     "data_pipelines",
	}, 1)
	if err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	publicTask, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      &folder.ID,
		NodeType:      string(TaskNodeTypeFile),
		Name:          "public_sync.env",
		Mode:          string(TaskModeBatch),
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env { parallelism = 1 }",
		Definition:    JSONMap{"is_public": true},
	}, 1)
	if err != nil {
		t.Fatalf("创建公开任务失败: %v", err)
	}

	// 使用独立任务验证共享权限接口，避免改变后续角色权限场景的前置数据。
	// Use a separate task for permission API coverage so later role assertions keep their fixture state.
	permissionTask, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      &folder.ID,
		NodeType:      string(TaskNodeTypeFile),
		Name:          "permission_api.env",
		Mode:          string(TaskModeBatch),
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env { parallelism = 1 }",
		Definition:    JSONMap{"is_public": true},
	}, 1)
	if err != nil {
		t.Fatalf("创建权限接口测试任务失败: %v", err)
	}

	ownerPermissions, err := service.GetTaskPermissionsForActor(ctx, executionapp.Actor{UserID: 1}, permissionTask.ID)
	if err != nil {
		t.Fatalf("任务所有者读取权限失败: %v", err)
	}
	if !ownerPermissions.CanManage || !ownerPermissions.IsOwner || !ownerPermissions.IsPublic {
		t.Fatalf("任务所有者权限结果错误: %+v", ownerPermissions)
	}
	if _, _, err := service.UpdateTaskPermissionsForActor(ctx, executionapp.Actor{UserID: 4}, permissionTask.ID, &UpdateTaskPermissionsRequest{IsPublic: func() *bool { value := false; return &value }()}); !errors.Is(err, ErrTaskPermissionDenied) {
		t.Fatalf("非所有者修改共享权限应被拒绝，得到: %v", err)
	}
	beforePermissions, afterPermissions, err := service.UpdateTaskPermissionsForActor(ctx, executionapp.Actor{UserID: 1}, permissionTask.ID, &UpdateTaskPermissionsRequest{
		IsPublic:        func() *bool { value := false; return &value }(),
		CollaboratorIDs: []uint{4},
	})
	if err != nil {
		t.Fatalf("任务所有者修改共享权限失败: %v", err)
	}
	if !beforePermissions.IsPublic || afterPermissions.IsPublic || len(afterPermissions.CollaboratorIDs) != 1 || afterPermissions.CollaboratorIDs[0] != 4 {
		t.Fatalf("共享权限修改前后结果错误: before=%+v after=%+v", beforePermissions, afterPermissions)
	}

	// 2. Test permission decoration for admin vs ordinary user
	adminActor := executionapp.Actor{UserID: 1, IsAdmin: true}
	regularActor := executionapp.Actor{UserID: 4, IsAdmin: false}

	adminTask, err := service.GetTaskForActor(ctx, adminActor, publicTask.ID)
	if err != nil {
		t.Fatalf("admin 读取公开任务失败: %v", err)
	}
	if !adminTask.CanEdit || !adminTask.CanRun || !adminTask.IsOwner {
		t.Fatalf("admin 应该拥有完整权限: %+v", adminTask)
	}

	regularTask, err := service.GetTaskForActor(ctx, regularActor, publicTask.ID)
	if err != nil {
		t.Fatalf("普通用户读取公开任务失败: %v", err)
	}
	if regularTask.CanEdit || regularTask.CanRun || regularTask.IsOwner {
		t.Fatalf("普通用户对他人公开任务应为只读锁定状态: %+v", regularTask)
	}

	// 3. Regular user attempting to edit or delete should be rejected
	_, err = service.UpdateTaskForActor(ctx, regularActor, publicTask.ID, &UpdateTaskRequest{
		Name:    "public_sync.env",
		Content: "env { parallelism = 2 }",
	})
	if !errors.Is(err, ErrTaskReadOnly) {
		t.Fatalf("普通用户更新他人公开任务应返回 ErrTaskReadOnly, got: %v", err)
	}

	err = service.DeleteTaskForActor(ctx, regularActor, publicTask.ID)
	if !errors.Is(err, ErrTaskPermissionDenied) {
		t.Fatalf("普通用户删除他人任务应返回 ErrTaskPermissionDenied, got: %v", err)
	}

	// 4. Create an admin job for this task, regular user should see it when querying by task_id
	adminJob := &JobInstance{
		TaskID:        publicTask.ID,
		TaskVersion:   1,
		RunType:       RunTypeRun,
		Status:        JobStatusRunning,
		PlatformJobID: "platform-public-1",
		CreatedBy:     1,
	}
	if err := service.repo.CreateJobInstance(ctx, adminJob); err != nil {
		t.Fatalf("创建 admin 作业失败: %v", err)
	}

	// Regular user queries jobs with task_id -> should see admin's job
	jobs, total, err := service.ListJobsForActor(ctx, regularActor, &JobFilter{TaskID: publicTask.ID, Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("普通用户查询任务运行历史失败: %v", err)
	}
	if total != 1 || len(jobs) != 1 || jobs[0].ID != adminJob.ID {
		t.Fatalf("普通用户应能查看该任务下的历史作业: total=%d, jobs=%+v", total, jobs)
	}

	// Regular user trying to cancel admin's job should be denied
	_, err = service.CancelJobForActor(ctx, regularActor, adminJob.ID, false)
	if !errors.Is(err, ErrTaskPermissionDenied) {
		t.Fatalf("普通用户取消他人作业应返回 ErrTaskPermissionDenied, got: %v", err)
	}

	// 5. Test Collaborator functionality
	// Add user 4 as collaborator
	publicTaskValue := true
	_, _, err = service.UpdateTaskPermissionsForActor(ctx, adminActor, publicTask.ID, &UpdateTaskPermissionsRequest{
		IsPublic:        &publicTaskValue,
		CollaboratorIDs: []uint{4},
	})
	if err != nil {
		t.Fatalf("添加共建者失败: %v", err)
	}
	// 即使管理员也必须通过权限接口修改共享字段。
	// Even an administrator must use the permission endpoint for sharing changes.
	adminContentTask, err := service.UpdateTaskForActor(ctx, adminActor, publicTask.ID, &UpdateTaskRequest{
		ParentID:   &folder.ID,
		Name:       "public_sync.env",
		Content:    "env { parallelism = 2 }",
		Definition: JSONMap{"is_public": false, "collaborators": []interface{}{}},
	})
	if err != nil {
		t.Fatalf("管理员更新任务正文失败: %v", err)
	}
	if adminContentTask.Definition["is_public"] != true || len(adminContentTask.CollaboratorIDs()) != 1 {
		t.Fatalf("管理员通过正文接口修改了共享权限: %+v", adminContentTask.Definition)
	}

	collabTask, err := service.GetTaskForActor(ctx, regularActor, publicTask.ID)
	if err != nil {
		t.Fatalf("共建者读取任务失败: %v", err)
	}
	if !collabTask.CanEdit || !collabTask.CanRun || !collabTask.IsCollaborator || collabTask.IsOwner {
		t.Fatalf("共建者应具有编辑与运行权限，但不是所有者: %+v", collabTask)
	}

	// Collaborator can update content
	updatedTask, err := service.UpdateTaskForActor(ctx, regularActor, publicTask.ID, &UpdateTaskRequest{
		ParentID:   &folder.ID,
		Name:       "public_sync.env",
		Content:    "env { parallelism = 4 }",
		Definition: JSONMap{"is_public": false, "collaborators": []interface{}{}}, // Try to tamper permissions
	})
	if err != nil {
		t.Fatalf("共建者更新任务内容失败: %v", err)
	}
	if updatedTask.Content != "env { parallelism = 4 }" {
		t.Fatalf("共建者更新内容未生效")
	}
	// Verify collaborator cannot tamper with is_public or collaborators
	if updatedTask.Definition["is_public"] != true || len(updatedTask.CollaboratorIDs()) != 1 {
		t.Fatalf("共建者篡改权限配置应该被忽略: %+v", updatedTask.Definition)
	}

	// 6. Test Private Task visibility
	privateTask, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID:      &folder.ID,
		NodeType:      string(TaskNodeTypeFile),
		Name:          "secret_sync.env",
		Mode:          string(TaskModeBatch),
		ContentFormat: string(ContentFormatHOCON),
		Content:       "env { parallelism = 1 }",
		Definition:    JSONMap{"is_public": false},
	}, 1)
	if err != nil {
		t.Fatalf("创建私有任务失败: %v", err)
	}

	// User 4 has no permission on privateTask
	otherUserActor := executionapp.Actor{UserID: 8, IsAdmin: false}
	_, err = service.GetTaskForActor(ctx, otherUserActor, privateTask.ID)
	if !errors.Is(err, ErrTaskPermissionDenied) {
		t.Fatalf("非所有者/非共建者访问私有任务应被拒绝: %v", err)
	}

	// Check tree filtering for otherUserActor
	tree, err := service.GetTaskTreeForActor(ctx, otherUserActor)
	if err != nil {
		t.Fatalf("获取任务树失败: %v", err)
	}
	var findTaskInTree func(nodes []*TaskTreeNode, targetID uint) bool
	findTaskInTree = func(nodes []*TaskTreeNode, targetID uint) bool {
		for _, n := range nodes {
			if n.ID == targetID {
				return true
			}
			if findTaskInTree(n.Children, targetID) {
				return true
			}
		}
		return false
	}
	if findTaskInTree(tree, privateTask.ID) {
		t.Fatalf("私有任务不应该出现在非授权用户的任务树中")
	}
}
