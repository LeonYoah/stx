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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDefaultConfigToolClientInspectDagAndPreview(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/config/dag", func(w http.ResponseWriter, r *http.Request) {
		var req ConfigToolContentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode dag request: %v", err)
		}
		if req.ContentFormat != "hocon" {
			t.Fatalf("expected hocon content format, got %s", req.ContentFormat)
		}
		_ = json.NewEncoder(w).Encode(ConfigToolDAGResponse{OK: true, Graph: ConfigToolGraph{Nodes: []map[string]interface{}{{"nodeId": "source-0"}}, Edges: []map[string]interface{}{{"fromNodeId": "source-0", "toNodeId": "sink-0"}}}})
	})
	mux.HandleFunc("/api/v1/config/webui-dag", func(w http.ResponseWriter, r *http.Request) {
		var req ConfigToolContentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode webui dag request: %v", err)
		}
		if req.ContentFormat != "hocon" {
			t.Fatalf("expected hocon content format for webui dag, got %s", req.ContentFormat)
		}
		_ = json.NewEncoder(w).Encode(ConfigToolWebUIDAGResponse{
			JobID:     "preview",
			JobName:   "Config Preview",
			JobStatus: "CREATED",
			JobDag: ConfigToolWebUIJobDAG{
				JobID: "preview",
				PipelineEdges: map[string][]ConfigToolWebUIDAGEdge{
					"0": {{InputVertexID: 1, TargetVertexID: 2}},
				},
				VertexInfoMap: map[string]ConfigToolWebUIDAGVertexInfo{
					"1": {VertexID: 1, Type: "source", ConnectorType: "Source[0]-FakeSource"},
					"2": {VertexID: 2, Type: "sink", ConnectorType: "Sink[0]-Console"},
				},
			},
			Metrics: map[string]interface{}{"SourceReceivedCount": "0"},
		})
	})
	mux.HandleFunc("/api/v1/config/validate", func(w http.ResponseWriter, r *http.Request) {
		var req ConfigToolValidateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode validate request: %v", err)
		}
		if !req.TestConnection {
			t.Fatalf("expected testConnection=true in validate request")
		}
		_ = json.NewEncoder(w).Encode(ConfigToolValidateResponse{
			OK:      true,
			Valid:   true,
			Summary: "Connection test finished.",
			Checks: []ConfigToolValidationCheck{{
				NodeID:        "source-0",
				ConnectorType: "Source[0]-Jdbc",
				Status:        "success",
				Message:       "Connection succeeded.",
			}},
		})
	})
	mux.HandleFunc("/api/v1/config/preview/source", func(w http.ResponseWriter, r *http.Request) {
		var req ConfigToolPreviewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode source preview request: %v", err)
		}
		if req.HttpSink["url"] != "https://preview.example.com" {
			t.Fatalf("unexpected http sink url: %#v", req.HttpSink["url"])
		}
		_ = json.NewEncoder(w).Encode(ConfigToolPreviewResponse{OK: true, Content: "env {}", ContentFormat: "hocon"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewDefaultConfigToolClient()
	if _, err := client.InspectDAG(context.Background(), server.URL, &ConfigToolContentRequest{Content: "env {}", ContentFormat: "hocon"}); err != nil {
		t.Fatalf("InspectDAG returned error: %v", err)
	}
	if _, err := client.InspectWebUIDAG(context.Background(), server.URL, &ConfigToolContentRequest{Content: "env {}", ContentFormat: "hocon"}); err != nil {
		t.Fatalf("InspectWebUIDAG returned error: %v", err)
	}
	if _, err := client.ValidateConfig(context.Background(), server.URL, &ConfigToolValidateRequest{ConfigToolContentRequest: ConfigToolContentRequest{Content: "env {}", ContentFormat: "hocon"}, TestConnection: true}); err != nil {
		t.Fatalf("ValidateConfig returned error: %v", err)
	}
	if _, err := client.DeriveSourcePreview(context.Background(), server.URL, &ConfigToolPreviewRequest{ConfigToolContentRequest: ConfigToolContentRequest{Content: "env {}", ContentFormat: "hocon"}, HttpSink: map[string]interface{}{"url": "https://preview.example.com"}}); err != nil {
		t.Fatalf("DeriveSourcePreview returned error: %v", err)
	}
}

func TestDefaultConfigToolClientParsesFriendlyConfigErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":      false,
			"message": "Config parse failed: String: 20: Expecting close brace } or a comma",
		})
	}))
	defer server.Close()

	client := NewDefaultConfigToolClient()
	_, err := client.InspectWebUIDAG(context.Background(), server.URL, &ConfigToolContentRequest{Content: "broken", ContentFormat: "hocon"})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "sync: 配置解析失败，请检查括号、引号、逗号和换行：String: 20: Expecting close brace } or a comma" {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDefaultConfigToolClientHumanizesResolutionErrors 测试各种数据库与连接器常见报错的全局人性化转译
// TestDefaultConfigToolClientHumanizesResolutionErrors verifies global humanization of common database & connector errors
func TestDefaultConfigToolClientHumanizesResolutionErrors(t *testing.T) {
	testCases := []struct {
		name       string
		path       string
		rawMsg     string
		expectedSub string
	}{
		{
			name:       "CDC catalog table not found",
			path:       "/api/v1/config/validate",
			rawMsg:     "Source[0]-MySQL-CDC official tablePath resolution failed: FactoryException: ErrorCode:[API-06], ErrorDescription:[Factory initialize failed] - Unable to create a source for identifier 'MySQL-CDC'. | caused by: SeaTunnelException: Can not find catalog table with factoryId [MySQL]",
			expectedSub: "目标数据库中未找到匹配的物理表元数据",
		},
		{
			name:       "JDBC table does not exist",
			path:       "/api/v1/config/validate",
			rawMsg:     "Source[0]-Jdbc official tablePath resolution failed: SQLSyntaxErrorException: Table 'stx_e2e.users_src' doesn't exist",
			expectedSub: "指定数据表在数据库中不存在",
		},
		{
			name:       "Access denied credential error",
			path:       "/api/v1/config/webui-dag",
			rawMsg:     "SQLException: Access denied for user 'root'@'localhost'",
			expectedSub: "数据库账号或密码认证失败",
		},
		{
			name:       "Network communication failure",
			path:       "/api/v1/config/validate",
			rawMsg:     "Communications link failure: Connection refused",
			expectedSub: "无法连接到目标数据库",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			humanized := humanizeConfigToolMessage(tc.path, tc.rawMsg)
			if !strings.Contains(humanized, tc.expectedSub) {
				t.Fatalf("expected error to contain %q, but got %q", tc.expectedSub, humanized)
			}
		})
	}
}
