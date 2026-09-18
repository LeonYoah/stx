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
	"testing"
	"time"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
)

func TestRequestSetsCLIHeadersAndDecodesData(t *testing.T) {
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
		if request.Header.Get("User-Agent") != "stx-cli/0.1.0" {
			t.Errorf("User-Agent 请求头错误: %q", request.Header.Get("User-Agent"))
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

func TestEndpointSupportsServerURLWithAPIPrefix(t *testing.T) {
	client, err := New(cliConfig.Resolved{Server: "https://example.test/api", Timeout: time.Second}, nil)
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	if got := client.endpoint("/api/v1/auth/cli/login"); got != "https://example.test/api/v1/auth/cli/login" {
		t.Fatalf("带 API 前缀的地址错误: %s", got)
	}
}
