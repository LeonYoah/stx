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
	"strings"
	"testing"
)

func TestParseDolphinVersionAndAPIFamily(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw     string
		family  string
		wantErr bool
	}{
		{raw: "", family: ""},
		{raw: "3.0.0", family: SchedulerAPIStyleProcess},
		{raw: "3.1.9", family: SchedulerAPIStyleProcess},
		{raw: "3.2.2", family: SchedulerAPIStyleProcess},
		{raw: "v3.2.0", family: SchedulerAPIStyleProcess},
		{raw: "3.3.1", family: SchedulerAPIStyleWorkflow},
		{raw: "v3.3.1-release", family: SchedulerAPIStyleWorkflow},
		{raw: "3.3.0-release", family: SchedulerAPIStyleWorkflow},
		{raw: "3.4.0", family: SchedulerAPIStyleWorkflow},
		{raw: "2.0.7", wantErr: true},
		{raw: "1.3.9", wantErr: true},
		{raw: "4.0.0", wantErr: true},
		{raw: "not-a-version", wantErr: true},
	}
	for _, tc := range cases {
		family, err := dolphinAPIFamilyFromVersion(tc.raw)
		if tc.wantErr {
			if !errors.Is(err, ErrDolphinUnsupportedVersion) {
				t.Fatalf("%q: expected unsupported version, got family=%s err=%v", tc.raw, family, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", tc.raw, err)
		}
		if family != tc.family {
			t.Fatalf("%q: family=%s want %s", tc.raw, family, tc.family)
		}
	}
}

func TestDecodeDolphinGraphReadsProcessAndWorkflowJSON(t *testing.T) {
	t.Parallel()
	processRaw, _ := json.Marshal(map[string]any{
		"processDefinition":       map[string]any{"code": 9, "name": "wf", "version": 2, "releaseState": "ONLINE"},
		"processTaskRelationList": []map[string]any{{"preTaskCode": 1, "postTaskCode": 88}},
		"taskDefinitionList":      []map[string]any{{"code": 88, "name": "stx", "taskType": "SHELL", "taskParams": map[string]any{"rawScript": "echo"}}},
	})
	workflowRaw, _ := json.Marshal(map[string]any{
		"workflowDefinition":       map[string]any{"code": 9, "name": "wf", "version": 2, "releaseState": "ONLINE"},
		"workflowTaskRelationList": []map[string]any{{"preTaskCode": 1, "postTaskCode": 88}},
		"taskDefinitionList":       []map[string]any{{"code": 88, "name": "stx", "taskType": "SHELL", "taskParams": map[string]any{"rawScript": "echo"}}},
	})
	for _, raw := range []json.RawMessage{processRaw, workflowRaw} {
		graph, err := decodeDolphinGraph(raw)
		if err != nil {
			t.Fatal(err)
		}
		if graph.Code != 9 || graph.Version != 2 || len(graph.Relations) != 1 || graph.Relations[0].PostTaskCode != 88 {
			t.Fatalf("decoded graph = %+v", graph)
		}
		if dolphinTaskRawScript(graph.Tasks[0]) != "echo" {
			t.Fatalf("raw script = %q", dolphinTaskRawScript(graph.Tasks[0]))
		}
	}
}

func TestDolphinClientProcessFamily31And32RoundTrip(t *testing.T) {
	for _, version := range []string{"3.1.9", "3.2.2"} {
		t.Run(version, func(t *testing.T) {
			server, fake := newDolphinFakeServer(t, dolphinFakeProcess)
			client := newDolphinClient(&SchedulerConnection{
				BaseURL: server.URL, Token: "t", APIStyle: SchedulerAPIStyleAuto, DolphinVersion: version,
			}, server.Client())
			ctx := context.Background()
			if _, err := client.ListProjects(ctx); err != nil {
				t.Fatalf("list projects: %v", err)
			}
			items, err := client.ListWorkflows(ctx, 7)
			if err != nil {
				t.Fatalf("list workflows: %v", err)
			}
			if len(items) != 1 || items[0].Code != 9 {
				t.Fatalf("workflows = %+v", items)
			}
			graph, err := client.GetDefinition(ctx, 7, 9)
			if err != nil {
				t.Fatalf("get definition: %v", err)
			}
			if len(graph.Relations) != 1 || graph.Relations[0].PreTaskCode != 1 {
				t.Fatalf("%s relations were not decoded: %+v", version, graph.Relations)
			}
			codes, err := client.GenTaskCodes(ctx, 7, 1)
			if err != nil || codes[0] != 88001 {
				t.Fatalf("gen-task-codes: %v %v", codes, err)
			}
			created, err := client.CreateDefinition(ctx, 7, graph)
			if err != nil || created.Code == 0 {
				t.Fatalf("create: %+v %v", created, err)
			}
			if err := client.Release(ctx, 7, created.Code, dolphinReleaseOnline); err != nil {
				t.Fatalf("release: %v", err)
			}
			if err := client.CreateSchedule(ctx, 7, created.Code, "0 0 * * * ?", "Asia/Shanghai"); err != nil {
				t.Fatalf("create schedule: %v", err)
			}
			if fake.lastForm.Get("processDefinitionCode") == "" || fake.lastForm.Get("workflowDefinitionCode") != "" {
				t.Fatalf("%s schedule form = %v", version, fake.lastForm)
			}
			if err := client.OnlineSchedules(ctx, 7, created.Code); err != nil {
				t.Fatalf("online schedules: %v", err)
			}
			if fake.lastScheduleQuery.Get("processDefinitionCode") == "" || fake.lastScheduleQuery.Get("workflowDefinitionCode") != "" {
				t.Fatalf("%s schedule query = %v", version, fake.lastScheduleQuery)
			}
			if fake.saw("workflow-definition") {
				t.Fatalf("%s client used workflow-definition: %v", version, fake.paths)
			}
			if !fake.saw("/process-definition") {
				t.Fatalf("%s client missed process-definition: %v", version, fake.paths)
			}
			if client.style != SchedulerAPIStyleProcess {
				t.Fatalf("style = %s", client.style)
			}
		})
	}
}

func TestDolphinClientWorkflowFamily33RoundTrip(t *testing.T) {
	server, fake := newDolphinFakeServer(t, dolphinFakeWorkflow)
	client := newDolphinClient(&SchedulerConnection{
		BaseURL: server.URL, Token: "t", APIStyle: SchedulerAPIStyleAuto, DolphinVersion: "3.3.1",
	}, server.Client())
	ctx := context.Background()
	graph, err := client.GetDefinition(ctx, 7, 9)
	if err != nil {
		t.Fatalf("get definition: %v", err)
	}
	if len(graph.Relations) != 1 {
		t.Fatalf("3.3 workflowTaskRelationList was not decoded: %+v", graph.Relations)
	}
	if _, err := client.CreateDefinition(ctx, 7, graph); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := client.CreateSchedule(ctx, 7, 9, "0 0 * * * ?", "Asia/Shanghai"); err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	if fake.lastForm.Get("workflowDefinitionCode") == "" || fake.lastForm.Get("processDefinitionCode") != "" {
		t.Fatalf("3.3 schedule form = %v", fake.lastForm)
	}
	if err := client.OnlineSchedules(ctx, 7, 9); err != nil {
		t.Fatalf("online schedules: %v", err)
	}
	if fake.lastScheduleQuery.Get("workflowDefinitionCode") == "" || fake.lastScheduleQuery.Get("processDefinitionCode") != "" {
		t.Fatalf("3.3 schedule query = %v", fake.lastScheduleQuery)
	}
	if fake.saw("process-definition") {
		t.Fatalf("3.3 client used process-definition: %v", fake.paths)
	}
	if !fake.saw("/workflow-definition") {
		t.Fatalf("3.3 client missed workflow-definition: %v", fake.paths)
	}
	if client.style != SchedulerAPIStyleWorkflow {
		t.Fatalf("style = %s", client.style)
	}
}

func TestDolphinClientAutoProbePrefers33ThenFallsBackTo32(t *testing.T) {
	workflowServer, workflowFake := newDolphinFakeServer(t, dolphinFakeWorkflow)
	workflowClient := newDolphinClient(&SchedulerConnection{BaseURL: workflowServer.URL, Token: "t", APIStyle: SchedulerAPIStyleAuto}, workflowServer.Client())
	if _, err := workflowClient.ListWorkflows(context.Background(), 7); err != nil {
		t.Fatalf("auto 3.3: %v", err)
	}
	if workflowClient.style != SchedulerAPIStyleWorkflow {
		t.Fatalf("auto 3.3 style = %s", workflowClient.style)
	}
	if workflowFake.count("workflow-definition") == 0 {
		t.Fatal("auto probe did not hit workflow-definition")
	}

	processServer, processFake := newDolphinFakeServer(t, dolphinFakeProcess)
	processClient := newDolphinClient(&SchedulerConnection{BaseURL: processServer.URL, Token: "t", APIStyle: SchedulerAPIStyleAuto}, processServer.Client())
	if _, err := processClient.ListWorkflows(context.Background(), 7); err != nil {
		t.Fatalf("auto 3.2: %v", err)
	}
	if processClient.style != SchedulerAPIStyleProcess {
		t.Fatalf("auto 3.2 style = %s", processClient.style)
	}
	if !processFake.saw("workflow-definition") || !processFake.saw("process-definition") {
		t.Fatalf("auto 3.2 should probe workflow then process: %v", processFake.paths)
	}
}

func TestDolphinClientRejectsV2ProjectList(t *testing.T) {
	server, _ := newDolphinFakeServer(t, dolphinFakeV2)
	client := newDolphinClient(&SchedulerConnection{BaseURL: server.URL, Token: "t", APIStyle: SchedulerAPIStyleAuto}, server.Client())
	_, err := client.ListProjects(context.Background())
	if !errors.Is(err, ErrDolphinUnsupportedVersion) {
		t.Fatalf("expected unsupported 2.x, got %v", err)
	}
}

func TestDolphinClientRejectsWhenNeither3xPathExists(t *testing.T) {
	server, _ := newDolphinFakeServer(t, dolphinFakeV2)
	client := newDolphinClient(&SchedulerConnection{BaseURL: server.URL, Token: "t", APIStyle: SchedulerAPIStyleAuto}, server.Client())
	_, err := client.ListWorkflows(context.Background(), 7)
	if !errors.Is(err, ErrDolphinUnsupportedVersion) {
		t.Fatalf("expected unsupported version, got %v", err)
	}
}

func TestNormalizeSchedulerConnectionRejectsNon3xVersion(t *testing.T) {
	_, err := normalizeSchedulerConnection(nil, &SchedulerConnectionRequest{
		Name: "ds", BaseURL: "http://dolphin:12345/dolphinscheduler", Token: "t", DolphinVersion: "2.0.7",
	})
	if !errors.Is(err, ErrDolphinUnsupportedVersion) {
		t.Fatalf("expected unsupported version, got %v", err)
	}
	item, err := normalizeSchedulerConnection(nil, &SchedulerConnectionRequest{
		Name: "ds", BaseURL: "http://dolphin:12345/dolphinscheduler", Token: "t", DolphinVersion: "3.3.1", APIStyle: SchedulerAPIStyleAuto,
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.APIStyle != SchedulerAPIStyleWorkflow {
		t.Fatalf("3.3 auto style = %s", item.APIStyle)
	}
}

func TestDolphinConsoleURLByFamily(t *testing.T) {
	processURL := dolphinConsoleURL("http://ds:12345/dolphinscheduler", 7, 9, SchedulerAPIStyleProcess)
	if !strings.Contains(processURL, "/#/projects/7/process-definition/list/9") {
		t.Fatalf("process console url = %s", processURL)
	}
	workflowURL := dolphinConsoleURL("http://ds:12345/dolphinscheduler", 7, 9, SchedulerAPIStyleWorkflow)
	if !strings.Contains(workflowURL, "/projects/7/workflow-definition/9") {
		t.Fatalf("workflow console url = %s", workflowURL)
	}
}

func TestLooksLikeDolphinV2Projects(t *testing.T) {
	if !looksLikeDolphinV2Projects(json.RawMessage(`[{"id":1,"name":"default"}]`)) {
		t.Fatal("expected 2.x array payload")
	}
	if looksLikeDolphinV2Projects(json.RawMessage(`{"totalList":[{"code":7,"name":"stg"}]}`)) {
		t.Fatal("3.x payload should not look like 2.x")
	}
}
