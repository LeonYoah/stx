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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
)

func TestPushDolphinReuseDetectsCycleAndSkipsWrite(t *testing.T) {
	var wrote bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "workflow-definition") && request.Method == http.MethodGet && !strings.Contains(request.URL.Path, "/1") {
			writeDolphinOK(writer, map[string]any{"totalList": []any{}})
			return
		}
		if strings.HasSuffix(request.URL.Path, "/process-definition") && request.Method == http.MethodGet {
			writeDolphinOK(writer, map[string]any{"totalList": []any{}})
			return
		}
		if strings.Contains(request.URL.Path, "/process-definition/9") && request.Method == http.MethodGet {
			writeDolphinOK(writer, map[string]any{
				"processDefinition": map[string]any{"code": 9, "name": "wf", "version": 3, "releaseState": "OFFLINE"},
				"taskDefinitionList": []map[string]any{
					{"code": 1, "name": "a", "taskType": "SHELL", "version": 1},
					{"code": 2, "name": "b", "taskType": "SHELL", "version": 1},
				},
				"processTaskRelationList": []map[string]any{
					{"preTaskCode": 1, "postTaskCode": 2},
				},
			})
			return
		}
		if request.Method == http.MethodPut || request.Method == http.MethodPost {
			wrote = true
		}
		writeDolphinOK(writer, map[string]any{})
	}))
	defer server.Close()

	service, task, connID := newBoundPushService(t, server)
	_, err := service.PushDolphin(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, task.ID, &DolphinPushRequest{
		Mode:         "reuse",
		ConnectionID: connID,
		ProjectCode:  7,
		WorkflowCode: 9,
		Upstreams:    []int64{2},
		Downstreams:  []int64{1},
		DryRun:       true,
	})
	if !errors.Is(err, ErrDolphinCycle) {
		t.Fatalf("expected cycle, got %v", err)
	}
	if wrote {
		t.Fatal("cycle check wrote to Dolphin")
	}
}

func TestPushDolphinVersionConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "workflow-definition") {
			http.NotFound(writer, request)
			return
		}
		if strings.Contains(request.URL.Path, "/process-definition/9") {
			writeDolphinOK(writer, map[string]any{
				"processDefinition":       map[string]any{"code": 9, "name": "wf", "version": 4, "releaseState": "ONLINE"},
				"taskDefinitionList":      []map[string]any{{"code": 88, "name": "node", "taskType": "SHELL", "version": 1, "taskParams": map[string]any{"rawScript": "old"}}},
				"processTaskRelationList": []map[string]any{},
			})
			return
		}
		if strings.HasSuffix(request.URL.Path, "/process-definition") {
			writeDolphinOK(writer, map[string]any{"totalList": []any{}})
			return
		}
		writeDolphinOK(writer, map[string]any{})
	}))
	defer server.Close()

	service, task, connID := newBoundPushService(t, server)
	if err := service.repo.SaveSchedulerBinding(context.Background(), &TaskSchedulerBinding{
		TaskID: task.ID, Kind: SchedulerKindDolphin, ConnectionID: connID, ProjectCode: 7, WorkflowCode: 9, TaskCode: 88,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	_, err := service.PushDolphin(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, task.ID, &DolphinPushRequest{
		Mode:            "update",
		ConnectionID:    connID,
		ExpectedVersion: 3,
	})
	if !errors.Is(err, ErrDolphinVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func TestPushDolphinUpdateDryRunKeepsEdges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "workflow-definition") {
			http.NotFound(writer, request)
			return
		}
		if strings.Contains(request.URL.Path, "/process-definition/9") {
			writeDolphinOK(writer, map[string]any{
				"processDefinition": map[string]any{"code": 9, "name": "wf", "version": 3, "releaseState": "ONLINE"},
				"taskDefinitionList": []map[string]any{
					{"code": 88, "name": "node", "taskType": "SHELL", "version": 1, "taskParams": map[string]any{"rawScript": "old"}},
				},
				"processTaskRelationList": []map[string]any{{"preTaskCode": 1, "postTaskCode": 88}},
			})
			return
		}
		if strings.HasSuffix(request.URL.Path, "/process-definition") {
			writeDolphinOK(writer, map[string]any{"totalList": []any{}})
			return
		}
		t.Fatalf("dry-run should not write: %s %s", request.Method, request.URL.Path)
	}))
	defer server.Close()

	service, task, connID := newBoundPushService(t, server)
	if err := service.repo.SaveSchedulerBinding(context.Background(), &TaskSchedulerBinding{
		TaskID: task.ID, Kind: SchedulerKindDolphin, ConnectionID: connID, ProjectCode: 7, WorkflowCode: 9, TaskCode: 88,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	result, err := service.PushDolphin(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, task.ID, &DolphinPushRequest{
		Mode: "update", ConnectionID: connID, DryRun: true,
	})
	if err != nil {
		t.Fatalf("dry-run update: %v", err)
	}
	if !result.WouldOffline || result.TaskCode != 88 {
		t.Fatalf("unexpected dry-run result: %+v", result)
	}
}

func TestDolphinClientSwitchesProcessAndWorkflowPaths(t *testing.T) {
	var sawProcess, sawWorkflow bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "workflow-definition") {
			sawWorkflow = true
			writeDolphinOK(writer, map[string]any{"totalList": []any{}})
			return
		}
		if strings.Contains(request.URL.Path, "process-definition") {
			sawProcess = true
			http.Error(writer, "missing", http.StatusNotFound)
			return
		}
		writeDolphinOK(writer, map[string]any{"totalList": []any{}})
	}))
	defer server.Close()

	client := newDolphinClient(&SchedulerConnection{BaseURL: server.URL, Token: "t", APIStyle: SchedulerAPIStyleAuto}, server.Client())
	if _, err := client.ListWorkflows(context.Background(), 1); err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if !sawWorkflow {
		t.Fatal("expected workflow-definition probe")
	}
	if client.style != SchedulerAPIStyleWorkflow {
		t.Fatalf("style = %s", client.style)
	}

	processServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "workflow-definition") {
			http.Error(writer, "missing", http.StatusNotFound)
			return
		}
		if strings.Contains(request.URL.Path, "process-definition") {
			sawProcess = true
			writeDolphinOK(writer, map[string]any{"totalList": []any{}})
			return
		}
		writeDolphinOK(writer, map[string]any{})
	}))
	defer processServer.Close()
	processClient := newDolphinClient(&SchedulerConnection{BaseURL: processServer.URL, Token: "t", APIStyle: SchedulerAPIStyleAuto}, processServer.Client())
	if _, err := processClient.ListWorkflows(context.Background(), 1); err != nil {
		t.Fatalf("list process workflows: %v", err)
	}
	if processClient.style != SchedulerAPIStyleProcess {
		t.Fatalf("process style = %s", processClient.style)
	}
	if !sawProcess {
		t.Fatal("expected process-definition probe")
	}
}

func TestPushDolphinNewWorkflowOn31And32UsesProcessPaths(t *testing.T) {
	for _, version := range []string{"3.1.9", "3.2.2"} {
		t.Run(version, func(t *testing.T) {
			server, fake := newDolphinFakeServer(t, dolphinFakeProcess)
			service, task, connID := newPushServiceWithDolphin(t, server, version, SchedulerAPIStyleAuto)
			result, err := service.PushDolphin(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, task.ID, &DolphinPushRequest{
				Mode: "new", ConnectionID: connID, ProjectCode: 7, WorkflowName: "orders",
			})
			if err != nil {
				t.Fatalf("push %s: %v", version, err)
			}
			if result.TaskCode == 0 || result.Binding == nil {
				t.Fatalf("result = %+v", result)
			}
			if fake.saw("workflow-definition") {
				t.Fatalf("%s push used 3.3 paths: %v", version, fake.paths)
			}
			if !fake.saw("POST /projects/7/process-definition") {
				t.Fatalf("%s push missed create: %v", version, fake.paths)
			}
			if version == "3.1.9" && !strings.Contains(result.Binding.ConsoleURL, "process-definition") {
				t.Fatalf("3.1 console url = %s", result.Binding.ConsoleURL)
			}
			if version == "3.2.2" && !strings.Contains(result.Binding.ConsoleURL, "workflow-definition") {
				t.Fatalf("3.2 console url = %s", result.Binding.ConsoleURL)
			}
		})
	}
}

func TestPushDolphinNewWorkflowOn33UsesWorkflowPaths(t *testing.T) {
	server, fake := newDolphinFakeServer(t, dolphinFakeWorkflow)
	service, task, connID := newPushServiceWithDolphin(t, server, "3.3.1", SchedulerAPIStyleAuto)
	result, err := service.PushDolphin(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, task.ID, &DolphinPushRequest{
		Mode: "new", ConnectionID: connID, ProjectCode: 7, WorkflowName: "orders",
	})
	if err != nil {
		t.Fatalf("push 3.3: %v", err)
	}
	if fake.saw("process-definition") {
		t.Fatalf("3.3 push used 3.2 paths: %v", fake.paths)
	}
	if !fake.saw("POST /projects/7/workflow-definition") {
		t.Fatalf("3.3 push missed create: %v", fake.paths)
	}
	if !strings.Contains(result.Binding.ConsoleURL, "workflow-definition") {
		t.Fatalf("console url = %s", result.Binding.ConsoleURL)
	}
}

func TestPushDolphinUpdateOn33KeepsWorkflowRelations(t *testing.T) {
	server, fake := newDolphinFakeServer(t, dolphinFakeWorkflow)
	service, task, connID := newPushServiceWithDolphin(t, server, "3.3.1", SchedulerAPIStyleAuto)
	if err := service.repo.SaveSchedulerBinding(context.Background(), &TaskSchedulerBinding{
		TaskID: task.ID, Kind: SchedulerKindDolphin, ConnectionID: connID, ProjectCode: 7, WorkflowCode: 9, TaskCode: 88,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	result, err := service.PushDolphin(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, task.ID, &DolphinPushRequest{
		Mode: "update", ConnectionID: connID,
	})
	if err != nil {
		t.Fatalf("update 3.3: %v", err)
	}
	if result.TaskCode != 88 {
		t.Fatalf("task code = %d", result.TaskCode)
	}
	if fake.saw("process-definition") {
		t.Fatalf("3.3 update used 3.2 paths: %v", fake.paths)
	}
	if !fake.saw("PUT /projects/7/workflow-definition/9") {
		t.Fatalf("3.3 update missed PUT definition: %v", fake.paths)
	}
	var relations []dolphinRelation
	if err := json.Unmarshal([]byte(fake.lastForm.Get("taskRelationJson")), &relations); err != nil {
		t.Fatalf("decode taskRelationJson: %v form=%v", err, fake.lastForm)
	}
	if len(relations) != 1 || relations[0].PreTaskCode != 1 || relations[0].PostTaskCode != 88 {
		t.Fatalf("3.3 update dropped workflowTaskRelationList: %+v", relations)
	}
}

func newPushServiceWithDolphin(t *testing.T, server *httptest.Server, version, style string) (*Service, *Task, uint) {
	t.Helper()
	service := newTestSyncService(t)
	service.SetDolphinHTTPClient(server.Client())
	ctx := context.Background()
	conn := &SchedulerConnection{
		Name: "ds", Kind: SchedulerKindDolphin, BaseURL: server.URL, Token: "secret", APIStyle: style, DolphinVersion: version,
	}
	if err := service.repo.CreateSchedulerConnection(ctx, conn); err != nil {
		t.Fatalf("create connection: %v", err)
	}
	folder, err := service.CreateTask(ctx, &CreateTaskRequest{NodeType: string(TaskNodeTypeFolder), Name: "root", ContentFormat: string(ContentFormatHOCON)}, 1)
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	task, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID: uintPtr(folder.ID), NodeType: string(TaskNodeTypeFile), Name: "orders", Mode: string(TaskModeBatch), ContentFormat: string(ContentFormatHOCON), Content: "env {}",
	}, 1)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return service, task, conn.ID
}

func newBoundPushService(t *testing.T, server *httptest.Server) (*Service, *Task, uint) {
	t.Helper()
	service := newTestSyncService(t)
	service.SetDolphinHTTPClient(server.Client())
	ctx := context.Background()
	conn := &SchedulerConnection{
		Name: "ds", Kind: SchedulerKindDolphin, BaseURL: server.URL, Token: "secret", APIStyle: SchedulerAPIStyleProcess,
	}
	if err := service.repo.CreateSchedulerConnection(ctx, conn); err != nil {
		t.Fatalf("create connection: %v", err)
	}
	folder, err := service.CreateTask(ctx, &CreateTaskRequest{NodeType: string(TaskNodeTypeFolder), Name: "root", ContentFormat: string(ContentFormatHOCON)}, 1)
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	task, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID: uintPtr(folder.ID), NodeType: string(TaskNodeTypeFile), Name: "orders", Mode: string(TaskModeBatch), ContentFormat: string(ContentFormatHOCON), Content: "env {}",
	}, 1)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return service, task, conn.ID
}

func writeDolphinOK(writer http.ResponseWriter, data any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "msg": "success", "data": data})
}
