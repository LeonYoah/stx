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

// newPlanDAGJob 构造一个已提交的集群作业，submitted_content 与线上作业 #26 同形。
// newPlanDAGJob builds a submitted cluster job whose submitted_content mirrors production job #26.
func newPlanDAGJob() *JobInstance {
	return &JobInstance{
		ID:     26,
		Status: JobStatusRunning,
		SubmitSpec: JSONMap{
			"cluster_id":        float64(1),
			"submitted_content": "env { job.mode = \"STREAMING\" }\nsource { Postgres-CDC {} }\nsink { Console {} }",
			"submitted_format":  "hocon",
		},
	}
}

func newPlanDAGWebUIResponse() *ConfigToolWebUIDAGResponse {
	return &ConfigToolWebUIDAGResponse{
		JobID: "plan",
		JobDag: ConfigToolWebUIJobDAG{
			PipelineEdges: map[string][]ConfigToolWebUIDAGEdge{"0": {{InputVertexID: 1, TargetVertexID: 2}}},
			VertexInfoMap: map[string]ConfigToolWebUIDAGVertexInfo{
				"1": {VertexID: 1, Type: "source", ConnectorType: "Source[0]-Postgres-CDC", TablePaths: []string{"db.public.orders"},
					TableColumns: map[string][]string{"db.public.orders": {"id", "amount"}}},
				"2": {VertexID: 2, Type: "sink", ConnectorType: "Sink[0]-Console", TablePaths: []string{"db.public.orders"}},
			},
		},
		Warnings: []string{"console sink has no save mode"},
	}
}

func TestGetJobPlanDAG_success(t *testing.T) {
	client := &stubConfigToolClient{webuiResp: newPlanDAGWebUIResponse()}
	service := &Service{configToolClient: client, configToolResolver: &stubConfigToolResolver{endpoint: "http://proxy"}}

	result, err := service.getJobPlanDAG(context.Background(), newPlanDAGJob())
	if err != nil {
		t.Fatalf("getJobPlanDAG: %v", err)
	}
	if !result.Available || result.Reason != "" || result.JobID != 26 {
		t.Fatalf("expected available result, got %+v", result)
	}
	// 用提交时的内容与格式请求 config tool，而不是任务当前草稿。
	// The config tool receives the submitted content and format, not the task's current draft.
	if client.lastWebUIReq == nil || !strings.Contains(client.lastWebUIReq.Content, "Postgres-CDC") || client.lastWebUIReq.ContentFormat != "hocon" {
		t.Fatalf("unexpected config tool request: %+v", client.lastWebUIReq)
	}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	webui, _ := decoded["webui_job"].(map[string]any)
	dag, _ := webui["jobDag"].(map[string]any)
	vertices, _ := dag["vertexInfoMap"].(map[string]any)
	source, _ := vertices["1"].(map[string]any)
	if source == nil || source["tableColumns"] == nil {
		t.Fatalf("webui_job must keep the WebUI-compatible jobDag with table columns, got %s", body)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected config tool warnings to pass through, got %v", result.Warnings)
	}
}

func TestGetJobPlanDAG_reasons(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(job *JobInstance, service *Service, client *stubConfigToolClient)
		reason  string
		message string
	}{
		{name: "local mode", reason: PlanDAGReasonLocalMode, mutate: func(job *JobInstance, _ *Service, _ *stubConfigToolClient) {
			job.SubmitSpec["execution_mode"] = "local"
		}},
		{name: "no submitted content", reason: PlanDAGReasonNoSubmittedContent, mutate: func(job *JobInstance, _ *Service, _ *stubConfigToolClient) {
			delete(job.SubmitSpec, "submitted_content")
		}},
		{name: "config tool not configured", reason: PlanDAGReasonConfigToolUnavailable, mutate: func(_ *JobInstance, service *Service, _ *stubConfigToolClient) {
			service.configToolClient = nil
		}},
		{name: "cluster endpoint unresolved", reason: PlanDAGReasonConfigToolUnavailable, mutate: func(_ *JobInstance, service *Service, _ *stubConfigToolClient) {
			service.configToolResolver = &stubConfigToolResolver{err: errors.New("cluster 1 not found")}
		}},
		{name: "inspect failed is redacted", reason: PlanDAGReasonInspectFailed, message: "password=******", mutate: func(_ *JobInstance, _ *Service, client *stubConfigToolClient) {
			client.webuiErr = errors.New("connect failed: password=secret123")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &stubConfigToolClient{webuiResp: newPlanDAGWebUIResponse()}
			service := &Service{configToolClient: client, configToolResolver: &stubConfigToolResolver{endpoint: "http://proxy"}}
			job := newPlanDAGJob()
			tc.mutate(job, service, client)

			result, err := service.getJobPlanDAG(context.Background(), job)
			if err != nil {
				t.Fatalf("getJobPlanDAG: %v", err)
			}
			if result.Available || result.Reason != tc.reason {
				t.Fatalf("got available=%v reason=%q, want %q", result.Available, result.Reason, tc.reason)
			}
			if result.Warnings == nil {
				t.Fatalf("warnings must never be nil")
			}
			if tc.message != "" && (!strings.Contains(result.Message, tc.message) || strings.Contains(result.Message, "secret123")) {
				t.Fatalf("message must be redacted, got %q", result.Message)
			}
		})
	}
}

func TestTruncatePlanDAGMessage(t *testing.T) {
	long := strings.Repeat("错", planDAGMessageMaxRunes+10)
	got := truncatePlanDAGMessage(long)
	if len([]rune(got)) != planDAGMessageMaxRunes+1 || !strings.HasSuffix(got, "…") {
		t.Fatalf("expected %d runes plus ellipsis, got %d", planDAGMessageMaxRunes, len([]rune(got)))
	}
	if truncatePlanDAGMessage("short") != "short" {
		t.Fatalf("short messages must stay unchanged")
	}
}
