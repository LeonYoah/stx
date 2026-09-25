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

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	stxversion "github.com/LeonYoah/stx/internal/version"
)

func TestRequestSetsCLIHeadersAndDecodesData(t *testing.T) {
	expectedUA := "stx-cli/" + stxversion.Version
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/test" {
			t.Errorf("请求路径 = %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer secret-token" {
			t.Errorf("Authorization 请求头错误: %q", request.Header.Get("Authorization"))
		}
		if request.Header.Get("X-STX-Client") != "cli" {
			t.Errorf("X-STX-Client 请求头错误: %q", request.Header.Get("X-STX-Client"))
		}
		if request.Header.Get("User-Agent") != expectedUA {
			t.Errorf("User-Agent 请求头错误: %q, want %q", request.Header.Get("User-Agent"), expectedUA)
		}
		if request.Header.Get("X-Request-ID") == "" {
			t.Error("缺少 X-Request-ID")
		}
		_ = json.NewEncoder(writer).Encode(Response{Data: json.RawMessage(`{"value":"ok"}`)})
	}))
	defer server.Close()

	client, err := New(cliConfig.Resolved{Server: server.URL, Token: "secret-token", Timeout: time.Second}, server.Client())
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	var result struct {
		Value string `json:"value"`
	}
	requestID, err := client.Request(context.Background(), http.MethodGet, "/api/v1/test", nil, &result)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if requestID == "" || result.Value != "ok" {
		t.Fatalf("请求结果错误: request_id=%q result=%+v", requestID, result)
	}
}

func TestRequestMapsHTTPStatusToStableExitCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(writer).Encode(Response{ErrorMsg: "需要管理员权限"})
	}))
	defer server.Close()

	client, err := New(cliConfig.Resolved{Server: server.URL, Timeout: time.Second}, server.Client())
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	_, err = client.Request(context.Background(), http.MethodGet, "/api/v1/test", nil, nil)
	if err == nil {
		t.Fatal("403 请求应该返回错误")
	}
	classified := clioutput.ClassifyError(err)
	if classified.Code != clioutput.CodePermission || classified.ExitCode != clioutput.ExitPermission {
		t.Fatalf("错误分类错误: code=%s exit=%d message=%s", classified.Code, classified.ExitCode, classified.Message)
	}
	if classified.RequestID == "" {
		t.Fatal("服务端错误应保留请求编号")
	}
}

func TestRequestDecodesLegacyBareJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"success":   true,
			"processes": []map[string]any{{"pid": 1234, "role": "master"}},
		})
	}))
	defer server.Close()

	client, err := New(cliConfig.Resolved{Server: server.URL, Timeout: time.Second}, server.Client())
	if err != nil {
		t.Fatalf("创建客户端失败 / creating client failed: %v", err)
	}
	var result struct {
		Success   bool `json:"success"`
		Processes []struct {
			PID  int    `json:"pid"`
			Role string `json:"role"`
		} `json:"processes"`
	}
	if _, err := client.Request(context.Background(), http.MethodPost, "/api/v1/test", nil, &result); err != nil {
		t.Fatalf("读取裸 JSON 响应失败 / decoding bare JSON response failed: %v", err)
	}
	if !result.Success || len(result.Processes) != 1 || result.Processes[0].PID != 1234 {
		t.Fatalf("裸 JSON 响应错误 / bare JSON response is incorrect: %#v", result)
	}
}

func TestRequestUsesLegacyBareErrorMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(writer).Encode(map[string]string{"error": "host not found / 主机不存在"})
	}))
	defer server.Close()

	client, err := New(cliConfig.Resolved{Server: server.URL, Timeout: time.Second}, server.Client())
	if err != nil {
		t.Fatalf("创建客户端失败 / creating client failed: %v", err)
	}
	_, err = client.Request(context.Background(), http.MethodPost, "/api/v1/test", nil, nil)
	classified := clioutput.ClassifyError(err)
	if classified.Code != clioutput.CodeNotFound || classified.ExitCode != clioutput.ExitNotFound || !strings.Contains(classified.Message, "host not found") {
		t.Fatalf("遗留错误响应映射错误 / legacy error response mapping is incorrect: %#v", classified)
	}
}

func TestRequestRejectsInvalidSuccessResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid json", body: `{"success":`},
		{name: "invalid envelope field", body: `{"data":{},"error_msg":[]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write([]byte(test.body))
			}))
			defer server.Close()

			client, err := New(cliConfig.Resolved{Server: server.URL, Timeout: time.Second}, server.Client())
			if err != nil {
				t.Fatalf("创建客户端失败 / creating client failed: %v", err)
			}
			var result any
			_, err = client.Request(context.Background(), http.MethodGet, "/api/v1/test", nil, &result)
			classified := clioutput.ClassifyError(err)
			if classified.Code != clioutput.CodeServer || classified.ExitCode != clioutput.ExitServer || !strings.Contains(classified.Message, "invalid STX response") {
				t.Fatalf("非法成功响应分类错误 / invalid success response classification is incorrect: %#v", classified)
			}
		})
	}
}

func TestEndpointSupportsServerURLWithAPIPrefix(t *testing.T) {
	client, err := New(cliConfig.Resolved{Server: "https://example.test/api", Timeout: time.Second}, nil)
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	if got := client.endpoint("/api/v1/auth/cli/login"); got != "https://example.test/api/v1/auth/cli/login" {
		t.Fatalf("带 API 前缀的地址错误: %s", got)
	}
}

func TestWithTimeoutReturnsIsolatedClient(t *testing.T) {
	originalHTTPClient := &http.Client{Timeout: time.Second}
	client, err := New(cliConfig.Resolved{Server: "https://example.test", Timeout: time.Second}, originalHTTPClient)
	if err != nil {
		t.Fatalf("创建客户端失败 / creating client failed: %v", err)
	}

	longRunning := client.WithTimeout(10 * time.Minute)
	if longRunning == client || longRunning.httpClient == client.httpClient {
		t.Fatal("长耗时客户端必须是独立副本 / long-running client must be an isolated copy")
	}
	if longRunning.timeout != 10*time.Minute || longRunning.httpClient.Timeout != 10*time.Minute {
		t.Fatalf("长耗时超时设置错误 / invalid long-running timeout: client=%s http=%s", longRunning.timeout, longRunning.httpClient.Timeout)
	}
	if client.timeout != time.Second || client.httpClient.Timeout != time.Second {
		t.Fatalf("原客户端不应被修改 / original client must remain unchanged: client=%s http=%s", client.timeout, client.httpClient.Timeout)
	}
}
