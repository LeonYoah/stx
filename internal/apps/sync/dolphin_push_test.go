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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestPushDolphinUpdatePreservesSiblingNodesAfterOffline(t *testing.T) {
	sparkNode := json.RawMessage(`{"code":11,"name":"spark_job","taskType":"SPARK","version":3,"taskGroupId":7,"taskGroupPriority":2,"cpuQuota":2,"taskParams":{"sparkVersion":"3.3","mainClass":"com.acme.Job","deployMode":"cluster"}}`)
	shellNode := json.RawMessage(`{"code":88,"name":"stx","taskType":"SHELL","version":1,"isCache":"NO","taskExecuteType":"BATCH","taskParams":{"rawScript":"old","localParams":[{"prop":"dt","direct":"IN","type":"VARCHAR","value":"x"}]}}`)
	conditionNode := json.RawMessage(`{"code":99,"name":"gate","taskType":"CONDITIONS","version":4,"flag":"YES","taskParams":{"conditionResult":{"successNode":["a"],"failedNode":["b"]}}}`)
	locations := `[{"taskCode":11,"x":12.5,"y":80,"extra":"pin"},{"taskCode":88,"x":240,"y":160},{"taskCode":99,"x":400,"y":80}]`
	relationJSON := json.RawMessage(`[{"id":55,"name":"e1","preTaskCode":11,"preTaskVersion":3,"postTaskCode":88,"postTaskVersion":1,"conditionType":"NONE","operator":1},{"preTaskCode":88,"preTaskVersion":1,"postTaskCode":99,"postTaskVersion":4,"conditionType":"NONE"}]`)
	gets := 0
	var putForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "workflow-definition") {
			http.NotFound(writer, request)
			return
		}
		if strings.HasSuffix(request.URL.Path, "/process-definition") && request.Method == http.MethodGet {
			writeDolphinOK(writer, map[string]any{"totalList": []any{}})
			return
		}
		if strings.Contains(request.URL.Path, "/process-definition/9") && request.Method == http.MethodGet {
			gets++
			payload := map[string]any{
				"processDefinition": map[string]any{
					"code": 9, "name": "wf", "version": 6, "releaseState": "ONLINE", "executionType": "SERIAL",
					"locations": locations, "globalParams": `[{"prop":"biz","value":"keep"}]`,
				},
				"taskDefinitionList": []json.RawMessage{sparkNode, shellNode, conditionNode},
			}
			var relations any
			_ = json.Unmarshal(relationJSON, &relations)
			payload["processTaskRelationList"] = relations
			if gets > 1 {
				payload["processDefinition"] = map[string]any{
					"code": 9, "name": "wf", "version": 6, "releaseState": "OFFLINE", "executionType": "SERIAL",
					"locations": "", "globalParams": `[{"prop":"biz","value":"keep"}]`,
				}
			}
			writeDolphinOK(writer, payload)
			return
		}
		if strings.Contains(request.URL.Path, "/process-definition/9") && request.Method == http.MethodPut {
			body, _ := io.ReadAll(request.Body)
			putForm, _ = url.ParseQuery(string(body))
			writeDolphinOK(writer, map[string]any{})
			return
		}
		if strings.Contains(request.URL.Path, "/release") {
			writeDolphinOK(writer, true)
			return
		}
		if strings.Contains(request.URL.Path, "/schedules") {
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
	if _, err := service.PushDolphin(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, task.ID, &DolphinPushRequest{
		Mode: "update", ConnectionID: connID,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if putForm == nil {
		t.Fatal("expected PUT definition")
	}
	var tasks []json.RawMessage
	if err := json.Unmarshal([]byte(putForm.Get("taskDefinitionJson")), &tasks); err != nil {
		t.Fatalf("tasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("update dropped sibling nodes: %s", putForm.Get("taskDefinitionJson"))
	}
	if string(tasks[0]) != string(sparkNode) {
		t.Fatalf("SPARK node was rewritten: %s", tasks[0])
	}
	if string(tasks[2]) != string(conditionNode) {
		t.Fatalf("CONDITIONS node was rewritten: %s", tasks[2])
	}
	if !strings.Contains(string(tasks[1]), `"rawScript"`) || strings.Contains(string(tasks[1]), `"rawScript":"old"`) {
		t.Fatalf("bound SHELL script was not updated: %s", tasks[1])
	}
	if !strings.Contains(string(tasks[1]), `"localParams"`) {
		t.Fatalf("bound SHELL lost taskParams extras: %s", tasks[1])
	}
	if !strings.Contains(putForm.Get("taskRelationJson"), `"id":55`) || !strings.Contains(putForm.Get("taskRelationJson"), `"operator":1`) {
		t.Fatalf("relation extras dropped: %s", putForm.Get("taskRelationJson"))
	}
	if !strings.Contains(putForm.Get("taskRelationJson"), `"postTaskVersion":2`) {
		t.Fatalf("bound relation version was not bumped: %s", putForm.Get("taskRelationJson"))
	}
	if !strings.Contains(putForm.Get("locations"), `"extra":"pin"`) || !strings.Contains(putForm.Get("locations"), `"taskCode":11`) {
		t.Fatalf("OFFLINE empty locations wiped canvas: %s", putForm.Get("locations"))
	}
	if putForm.Get("executionType") != "SERIAL" {
		t.Fatalf("executionType overwritten: %s", putForm.Get("executionType"))
	}
	if !strings.Contains(putForm.Get("globalParams"), `"biz"`) {
		t.Fatalf("globalParams dropped: %s", putForm.Get("globalParams"))
	}
}

func TestPushDolphinReusePreservesExistingGraph(t *testing.T) {
	existingTask := json.RawMessage(`{"code":11,"name":"up","taskType":"SPARK","version":2,"taskGroupId":9,"taskParams":{"mainClass":"Keep"}}`)
	var putForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "workflow-definition") {
			http.NotFound(writer, request)
			return
		}
		if strings.Contains(request.URL.Path, "gen-task-codes") {
			writeDolphinOK(writer, []int64{88001})
			return
		}
		if strings.HasSuffix(request.URL.Path, "/process-definition") && request.Method == http.MethodGet {
			writeDolphinOK(writer, map[string]any{"totalList": []any{}})
			return
		}
		if strings.Contains(request.URL.Path, "/process-definition/9") && request.Method == http.MethodGet {
			writeDolphinOK(writer, map[string]any{
				"processDefinition": map[string]any{
					"code": 9, "name": "wf", "version": 3, "releaseState": "OFFLINE",
					"locations": `[{"taskCode":11,"x":10,"y":20,"name":"up"}]`,
				},
				"taskDefinitionList":      []json.RawMessage{existingTask},
				"processTaskRelationList": []map[string]any{{"preTaskCode": 0, "postTaskCode": 11, "id": 7}},
			})
			return
		}
		if request.Method == http.MethodPut {
			body, _ := io.ReadAll(request.Body)
			putForm, _ = url.ParseQuery(string(body))
			writeDolphinOK(writer, map[string]any{})
			return
		}
		writeDolphinOK(writer, map[string]any{})
	}))
	defer server.Close()

	service, task, connID := newBoundPushService(t, server)
	if _, err := service.PushDolphin(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, task.ID, &DolphinPushRequest{
		Mode: "reuse", ConnectionID: connID, ProjectCode: 7, WorkflowCode: 9, Upstreams: []int64{11},
	}); err != nil {
		t.Fatalf("reuse: %v", err)
	}
	var tasks []json.RawMessage
	if err := json.Unmarshal([]byte(putForm.Get("taskDefinitionJson")), &tasks); err != nil {
		t.Fatalf("tasks: %v", err)
	}
	if len(tasks) != 2 || string(tasks[0]) != string(existingTask) {
		t.Fatalf("reuse rewrote existing node: %s", putForm.Get("taskDefinitionJson"))
	}
	if !strings.Contains(putForm.Get("locations"), `"name":"up"`) || !strings.Contains(putForm.Get("locations"), `"taskCode":88001`) {
		t.Fatalf("reuse locations = %s", putForm.Get("locations"))
	}
	if !strings.Contains(putForm.Get("taskRelationJson"), `"id":7`) {
		t.Fatalf("reuse dropped relation extras: %s", putForm.Get("taskRelationJson"))
	}
}

func TestPushDolphinUpdateRestoresSiblingsWhenOfflineGetDropsThem(t *testing.T) {
	sparkNode := json.RawMessage(`{"code":11,"name":"spark_job","taskType":"SPARK","version":3,"taskGroupId":7,"taskParams":{"sparkVersion":"3.3"}}`)
	shellNode := json.RawMessage(`{"code":88,"name":"stx","taskType":"SHELL","version":1,"isCache":"NO","taskExecuteType":"BATCH","taskParams":{"rawScript":"old"}}`)
	conditionNode := json.RawMessage(`{"code":99,"name":"gate","taskType":"CONDITIONS","version":4,"taskParams":{}}`)
	locations := `[{"taskCode":11,"x":12.5,"y":80,"extra":"pin"},{"taskCode":88,"x":240,"y":160},{"taskCode":99,"x":400,"y":80}]`
	gets := 0
	var putForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "workflow-definition") {
			http.NotFound(writer, request)
			return
		}
		if strings.HasSuffix(request.URL.Path, "/process-definition") && request.Method == http.MethodGet {
			writeDolphinOK(writer, map[string]any{"totalList": []any{}})
			return
		}
		if strings.Contains(request.URL.Path, "/process-definition/9") && request.Method == http.MethodGet {
			gets++
			payload := map[string]any{
				"processDefinition": map[string]any{
					"code": 9, "name": "wf", "version": 6, "releaseState": "ONLINE", "executionType": "SERIAL",
					"locations": locations, "globalParams": `[{"prop":"biz","value":"keep"}]`,
				},
				"taskDefinitionList":      []json.RawMessage{sparkNode, shellNode, conditionNode},
				"processTaskRelationList": []map[string]any{{"preTaskCode": 11, "preTaskVersion": 3, "postTaskCode": 88, "postTaskVersion": 1, "id": 55}},
			}
			if gets > 1 {
				// 模拟 3.2 OFFLINE 后再 GET：locations 被清空，兄弟节点也丢了。
				// Simulate 3.2 GET after OFFLINE: locations emptied and sibling nodes dropped.
				payload["processDefinition"] = map[string]any{
					"code": 9, "name": "wf", "version": 6, "releaseState": "OFFLINE",
					"locations": "", "globalParams": "[]",
				}
				payload["taskDefinitionList"] = []json.RawMessage{shellNode}
				payload["processTaskRelationList"] = []any{}
			}
			writeDolphinOK(writer, payload)
			return
		}
		if strings.Contains(request.URL.Path, "/process-definition/9") && request.Method == http.MethodPut {
			body, _ := io.ReadAll(request.Body)
			putForm, _ = url.ParseQuery(string(body))
			writeDolphinOK(writer, map[string]any{})
			return
		}
		if strings.Contains(request.URL.Path, "/release") || strings.Contains(request.URL.Path, "/schedules") {
			writeDolphinOK(writer, true)
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
	if _, err := service.PushDolphin(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, task.ID, &DolphinPushRequest{
		Mode: "update", ConnectionID: connID,
	}); err != nil {
		t.Fatalf("update: %v", err)
	}
	var tasks []json.RawMessage
	if err := json.Unmarshal([]byte(putForm.Get("taskDefinitionJson")), &tasks); err != nil {
		t.Fatalf("tasks: %v", err)
	}
	if len(tasks) != 3 || string(tasks[0]) != string(sparkNode) || string(tasks[2]) != string(conditionNode) {
		t.Fatalf("incomplete OFFLINE GET dropped siblings on PUT: %s", putForm.Get("taskDefinitionJson"))
	}
	if !strings.Contains(putForm.Get("locations"), `"extra":"pin"`) {
		t.Fatalf("incomplete OFFLINE GET wiped locations: %s", putForm.Get("locations"))
	}
	if !strings.Contains(putForm.Get("globalParams"), `"biz"`) {
		t.Fatalf("incomplete OFFLINE GET wiped globalParams: %s", putForm.Get("globalParams"))
	}
	if !strings.Contains(putForm.Get("taskRelationJson"), `"id":55`) {
		t.Fatalf("incomplete OFFLINE GET wiped relations: %s", putForm.Get("taskRelationJson"))
	}
}

func TestPushDolphinReuseRestoresGraphAfterOnlineOffline(t *testing.T) {
	existingTask := json.RawMessage(`{"code":11,"name":"up","taskType":"SPARK","version":2,"taskGroupId":9,"taskParams":{"mainClass":"Keep"}}`)
	locations := `[{"taskCode":11,"x":10,"y":20,"name":"up"}]`
	gets := 0
	var putForm url.Values
	wrotePut := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "workflow-definition") {
			http.NotFound(writer, request)
			return
		}
		if strings.Contains(request.URL.Path, "gen-task-codes") {
			writeDolphinOK(writer, []int64{88001})
			return
		}
		if strings.HasSuffix(request.URL.Path, "/process-definition") && request.Method == http.MethodGet {
			writeDolphinOK(writer, map[string]any{"totalList": []any{}})
			return
		}
		if strings.Contains(request.URL.Path, "/process-definition/9") && request.Method == http.MethodGet {
			gets++
			payload := map[string]any{
				"processDefinition": map[string]any{
					"code": 9, "name": "wf", "version": 3, "releaseState": "ONLINE",
					"locations": locations, "globalParams": `[{"prop":"k","value":"v"}]`,
				},
				"taskDefinitionList":      []json.RawMessage{existingTask},
				"processTaskRelationList": []map[string]any{{"preTaskCode": 0, "postTaskCode": 11, "id": 7}},
			}
			if gets > 1 {
				payload["processDefinition"] = map[string]any{
					"code": 9, "name": "wf", "version": 3, "releaseState": "OFFLINE", "locations": "[]",
				}
				payload["taskDefinitionList"] = []json.RawMessage{}
				payload["processTaskRelationList"] = []any{}
			}
			writeDolphinOK(writer, payload)
			return
		}
		if request.Method == http.MethodPut {
			wrotePut = true
			body, _ := io.ReadAll(request.Body)
			putForm, _ = url.ParseQuery(string(body))
			writeDolphinOK(writer, map[string]any{})
			return
		}
		if strings.Contains(request.URL.Path, "/release") || strings.Contains(request.URL.Path, "/schedules") {
			writeDolphinOK(writer, true)
			return
		}
		writeDolphinOK(writer, map[string]any{})
	}))
	defer server.Close()

	service, task, connID := newBoundPushService(t, server)
	if _, err := service.PushDolphin(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, task.ID, &DolphinPushRequest{
		Mode: "reuse", ConnectionID: connID, ProjectCode: 7, WorkflowCode: 9, Upstreams: []int64{11},
	}); err != nil {
		t.Fatalf("reuse: %v", err)
	}
	if !wrotePut {
		t.Fatal("expected PUT definition")
	}
	var tasks []json.RawMessage
	if err := json.Unmarshal([]byte(putForm.Get("taskDefinitionJson")), &tasks); err != nil {
		t.Fatalf("tasks: %v", err)
	}
	if len(tasks) != 2 || string(tasks[0]) != string(existingTask) {
		t.Fatalf("reuse after OFFLINE dropped existing node: %s", putForm.Get("taskDefinitionJson"))
	}
	if !strings.Contains(putForm.Get("locations"), `"name":"up"`) || !strings.Contains(putForm.Get("locations"), `"taskCode":88001`) {
		t.Fatalf("reuse after OFFLINE locations = %s", putForm.Get("locations"))
	}
	if !strings.Contains(putForm.Get("taskRelationJson"), `"id":7`) {
		t.Fatalf("reuse after OFFLINE dropped relation extras: %s", putForm.Get("taskRelationJson"))
	}
	if !strings.Contains(putForm.Get("globalParams"), `"k"`) {
		t.Fatalf("reuse after OFFLINE dropped globalParams: %s", putForm.Get("globalParams"))
	}
}

func TestRestoreGraphForWriteBackKeepsSavedPayload(t *testing.T) {
	saved := &dolphinGraph{
		Code: 9, Name: "wf", TenantCode: "default", ExecutionType: "SERIAL", Timeout: 30,
		GlobalParams: `[{"prop":"biz","value":"keep"}]`,
		Locations:    `[{"taskCode":11,"x":1,"y":2,"extra":"pin"}]`,
		Tasks:        []json.RawMessage{json.RawMessage(`{"code":11,"taskGroupId":7}`), json.RawMessage(`{"code":88}`)},
		Relations:    []json.RawMessage{json.RawMessage(`{"id":55,"preTaskCode":11,"postTaskCode":88}`)},
	}
	refreshed := &dolphinGraph{
		Code: 9, Version: 8, ReleaseState: dolphinReleaseOffline, Name: "wf",
		GlobalParams: "[]", Locations: "[]",
		Tasks:     []json.RawMessage{json.RawMessage(`{"code":88}`)},
		Relations: []json.RawMessage{},
	}
	restoreGraphForWriteBack(refreshed, saved)
	if err := assertGraphSafeToWrite(refreshed, saved); err != nil {
		t.Fatalf("restored graph rejected: %v", err)
	}
	if len(refreshed.Tasks) != 2 || string(refreshed.Tasks[0]) != `{"code":11,"taskGroupId":7}` {
		t.Fatalf("tasks not restored: %s", refreshed.Tasks)
	}
	if !strings.Contains(refreshed.Locations, `"extra":"pin"`) {
		t.Fatalf("locations not restored: %s", refreshed.Locations)
	}
	if refreshed.ExecutionType != "SERIAL" || !strings.Contains(refreshed.GlobalParams, `"biz"`) {
		t.Fatalf("metadata not restored: %+v", refreshed)
	}
	if err := assertGraphSafeToWrite(&dolphinGraph{Tasks: []json.RawMessage{json.RawMessage(`{"code":88}`)}}, saved); err == nil {
		t.Fatal("expected refusal when PUT would drop a sibling node")
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

func TestMergeDolphinLocationKeepsExistingNodes(t *testing.T) {
	merged := mergeDolphinLocation(`{"11":{"x":240,"y":160,"targetarr":"","name":"up"}}`, 22)
	var locs map[string]json.RawMessage
	if err := json.Unmarshal([]byte(merged), &locs); err != nil {
		t.Fatalf("unmarshal locations: %v", err)
	}
	if !strings.Contains(string(locs["11"]), `"targetarr"`) || locs["22"] == nil {
		t.Fatalf("expected extra keys and both nodes, got %s", merged)
	}
	arrayMerged := mergeDolphinLocation(`[{"taskCode":11,"x":240.5,"y":160,"extra":"keep"}]`, 22)
	if !strings.Contains(arrayMerged, `"extra":"keep"`) || !strings.Contains(arrayMerged, `"taskCode":22`) {
		t.Fatalf("array merge dropped fields: %s", arrayMerged)
	}
	empty := mergeDolphinLocation("{}", 33)
	if !strings.Contains(empty, `"taskCode":33`) {
		t.Fatalf("empty locations should still place the new node: %s", empty)
	}
	invalid := mergeDolphinLocation(`not-json`, 44)
	if invalid != "not-json" {
		t.Fatalf("invalid locations must not be rewritten: %s", invalid)
	}
}

func writeDolphinOK(writer http.ResponseWriter, data any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "msg": "success", "data": data})
}
