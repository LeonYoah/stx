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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildEngineURLWithContextPath(t *testing.T) {
	url, err := buildEngineURL(&EngineEndpoint{BaseURL: "http://127.0.0.1:8080", ContextPath: "/seatunnel"}, "/submit-job", map[string]string{"format": "sql", "jobName": "demo"})
	if err != nil {
		t.Fatalf("buildEngineURL returned error: %v", err)
	}
	want := "http://127.0.0.1:8080/seatunnel/submit-job?format=sql&jobName=demo"
	if url != want {
		t.Fatalf("expected %s, got %s", want, url)
	}
}

func TestSeaTunnelEngineClientSubmitGetAndStop(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/submit-job", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("format"); got != "sql" {
			t.Fatalf("expected format=sql, got %s", got)
		}
		if got := r.URL.Query().Get("jobName"); got != "demo-task" {
			t.Fatalf("expected jobName=demo-task, got %s", got)
		}
		_ = json.NewEncoder(w).Encode(EngineSubmitResponse{JobID: "123", JobName: "demo-task"})
	})
	mux.HandleFunc("/job-info/123", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(EngineJobInfo{JobID: "123", JobName: "demo-task", JobStatus: "RUNNING"})
	})
	mux.HandleFunc("/stop-job", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read stop body: %v", err)
		}
		var request map[string]interface{}
		if err := json.Unmarshal(payload, &request); err != nil {
			t.Fatalf("failed to decode stop body: %v", err)
		}
		if got := request["jobId"]; got != "123" {
			t.Fatalf("expected jobId=123, got %#v", got)
		}
		if got := request["isStopWithSavePoint"]; got != true {
			t.Fatalf("expected isStopWithSavePoint=true, got %#v", got)
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/logs/123", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><ul><li><a href="http://127.0.0.1:18081/logs/job-123.log">job-123.log</a></li></ul></body></html>`))
	})
	mux.HandleFunc("/logs/job-123.log", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("line1\nline2"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewSeaTunnelEngineClient()
	endpoint := &EngineEndpoint{BaseURL: server.URL}

	submitResp, err := client.Submit(context.Background(), &EngineSubmitRequest{Endpoint: endpoint, Format: "sql", JobName: "demo-task", Body: []byte("select 1")})
	if err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	if submitResp.JobID != "123" {
		t.Fatalf("expected job id 123, got %s", submitResp.JobID)
	}

	jobInfo, err := client.GetJobInfo(context.Background(), endpoint, "123")
	if err != nil {
		t.Fatalf("GetJobInfo returned error: %v", err)
	}
	if jobInfo.JobStatus != "RUNNING" {
		t.Fatalf("expected RUNNING, got %s", jobInfo.JobStatus)
	}

	if err := client.StopJob(context.Background(), endpoint, "123", true); err != nil {
		t.Fatalf("StopJob returned error: %v", err)
	}
	logs, err := client.GetJobLogs(context.Background(), endpoint, "123")
	if err != nil {
		t.Fatalf("GetJobLogs returned error: %v", err)
	}
	if logs != "line1\nline2" {
		t.Fatalf("expected resolved log content, got %q", logs)
	}
}

func TestSeaTunnelEngineClientFallsBackToLegacyV1(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/submit-job", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "<h1>404 Not Found</h1>No context found for request", http.StatusNotFound)
	})
	mux.HandleFunc("/hazelcast/rest/maps/submit-job", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("jobId"); got != "733" {
			t.Fatalf("expected jobId=733, got %s", got)
		}
		_ = json.NewEncoder(w).Encode(EngineSubmitResponse{JobID: "733", JobName: "legacy-demo"})
	})
	mux.HandleFunc("/hazelcast/rest/maps/job-info/733", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(EngineJobInfo{JobID: "733", JobName: "legacy-demo", JobStatus: "FINISHED"})
	})
	mux.HandleFunc("/hazelcast/rest/maps/stop-job", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("jobId"); got != "733" {
			t.Fatalf("expected legacy stop jobId=733, got %s", got)
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/hazelcast/rest/maps/logs/733", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><ul><li><a href="http://127.0.0.1:5801/hazelcast/rest/maps/logs/job-733.log">job-733.log</a></li></ul></body></html>`))
	})
	mux.HandleFunc("/hazelcast/rest/maps/logs/job-733.log", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("legacy-log-line"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewSeaTunnelEngineClient()
	endpoint := &EngineEndpoint{BaseURL: server.URL, LegacyURL: server.URL}

	submitResp, err := client.Submit(context.Background(), &EngineSubmitRequest{
		Endpoint: endpoint,
		Format:   "json",
		JobID:    "733",
		JobName:  "legacy-demo",
		Body:     []byte(`{"env":{"job.mode":"batch"}}`),
	})
	if err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	if submitResp.APIMode != "v1" {
		t.Fatalf("expected api mode v1, got %s", submitResp.APIMode)
	}

	jobInfo, err := client.GetJobInfo(context.Background(), &EngineEndpoint{BaseURL: server.URL, LegacyURL: server.URL, APIMode: "v1"}, "733")
	if err != nil {
		t.Fatalf("GetJobInfo legacy returned error: %v", err)
	}
	if jobInfo.JobStatus != "FINISHED" {
		t.Fatalf("expected FINISHED, got %s", jobInfo.JobStatus)
	}

	if err := client.StopJob(context.Background(), &EngineEndpoint{BaseURL: server.URL, LegacyURL: server.URL, APIMode: "v1"}, "733", true); err != nil {
		t.Fatalf("StopJob legacy returned error: %v", err)
	}
	logs, err := client.GetJobLogs(context.Background(), &EngineEndpoint{BaseURL: server.URL, LegacyURL: server.URL, APIMode: "v1"}, "733")
	if err != nil {
		t.Fatalf("GetJobLogs legacy returned error: %v", err)
	}
	if logs != "legacy-log-line" {
		t.Fatalf("expected resolved legacy log content, got %q", logs)
	}
}

// TestSeaTunnelEngineClient_SeaTunnel30_JobInfoWithMultiTableDagAndMetrics 验证 SeaTunnel 3.0 多表拓扑与表级指标解析
// TestSeaTunnelEngineClient_SeaTunnel30_JobInfoWithMultiTableDagAndMetrics verifies SeaTunnel 3.0 multi-table DAG and table-level metrics parsing
func TestSeaTunnelEngineClient_SeaTunnel30_JobInfoWithMultiTableDagAndMetrics(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/job-info/999", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 模拟 SeaTunnel 3.0 REST V2 返回的 JSON 结构 (vertexInfoMap 为数组，包含 tablePaths，指标细化到表)
		// Simulate SeaTunnel 3.0 REST V2 JSON response (vertexInfoMap as array with tablePaths and per-table metrics)
		raw30JobInfo := `{
			"jobId": "999",
			"jobName": "seatunnel-30-multi-table",
			"jobStatus": "RUNNING",
			"createTime": "2026-09-22 10:00:00",
			"finishTime": "",
			"jobDag": {
				"jobId": "999",
				"pipelineEdges": {
					"1": [
						{"inputVertexId": "1", "targetVertexId": "2"}
					]
				},
				"vertexInfoMap": [
					{
						"vertexId": 1,
						"type": "SOURCE",
						"vertexName": "connector-cdc-mysql",
						"tablePaths": ["mydb.users", "mydb.orders"]
					},
					{
						"vertexId": 2,
						"type": "SINK",
						"vertexName": "connector-jdbc",
						"tablePaths": ["sinkdb.users", "sinkdb.orders"]
					}
				]
			},
			"metrics": {
				"TableSourceReceivedCount": {
					"mydb.users": 1500,
					"mydb.orders": 3200
				},
				"TableSinkWriteCount": {
					"sinkdb.users": 1500,
					"sinkdb.orders": 3198
				},
				"TableSourceReceivedQPS": {
					"mydb.users": 150.5,
					"mydb.orders": 320.0
				},
				"TableSinkWriteQPS": {
					"sinkdb.users": 150.0,
					"sinkdb.orders": 319.5
				}
			}
		}`
		_, _ = w.Write([]byte(raw30JobInfo))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewSeaTunnelEngineClient()
	endpoint := &EngineEndpoint{BaseURL: server.URL, APIMode: "v2"}

	jobInfo, err := client.GetJobInfo(context.Background(), endpoint, "999")
	if err != nil {
		t.Fatalf("GetJobInfo returned error: %v", err)
	}

	if jobInfo.JobID != "999" || jobInfo.JobStatus != "RUNNING" {
		t.Fatalf("unexpected job info: %+v", jobInfo)
	}

	// 验证 jobDag 中包含 vertexInfoMap 数组结构
	// Verify jobDag contains vertexInfoMap array structure
	vertices, ok := jobInfo.JobDag["vertexInfoMap"].([]interface{})
	if !ok || len(vertices) != 2 {
		t.Fatalf("expected 2 vertices in vertexInfoMap, got: %v", jobInfo.JobDag["vertexInfoMap"])
	}

	v1, ok := vertices[0].(map[string]interface{})
	if !ok || v1["vertexName"] != "connector-cdc-mysql" {
		t.Fatalf("expected vertexName connector-cdc-mysql, got: %v", vertices[0])
	}
	tablePaths, ok := v1["tablePaths"].([]interface{})
	if !ok || len(tablePaths) != 2 {
		t.Fatalf("expected 2 table paths, got: %v", v1["tablePaths"])
	}

	// 验证 metrics 中正确反序列化表级指标映射
	// Verify metrics correctly deserializes per-table metric maps
	srcTableCounts, ok := jobInfo.Metrics["TableSourceReceivedCount"].(map[string]interface{})
	if !ok || len(srcTableCounts) != 2 {
		t.Fatalf("expected 2 table counts in TableSourceReceivedCount, got: %v", jobInfo.Metrics["TableSourceReceivedCount"])
	}

	// 验证 mergeJobRuntimeInfo 正确将 3.0 多表拓扑与指标合并入 ResultPreview
	// Verify mergeJobRuntimeInfo properly incorporates 3.0 multi-table DAG and metrics into ResultPreview
	merged := mergeJobRuntimeInfo(nil, jobInfo)
	if merged["job_status"] != "RUNNING" {
		t.Fatalf("expected RUNNING in merged result, got %v", merged["job_status"])
	}
	if merged["job_dag"] == nil || merged["metrics"] == nil {
		t.Fatalf("expected job_dag and metrics to be preserved in merged result")
	}
}
