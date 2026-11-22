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

package executor

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// engineHealthServerPort 起一个不验证路径的 httptest 服务并返回其端口。
// engineHealthServerPort starts a path-agnostic httptest server and returns its port.
func engineHealthServerPort(t *testing.T, status int, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hazelcast/rest/maps/overview" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("split %s: %v", srv.URL, err)
	}
	return port
}

// TestCheckEngineHealthHealthy 正常引擎：maps/overview 200 → known=true, healthy=true。
// TestCheckEngineHealthHealthy: a healthy engine answers 200 on maps/overview -> known+healthy.
func TestCheckEngineHealthHealthy(t *testing.T) {
	port := engineHealthServerPort(t, http.StatusOK, `{"seatunnel:engine:jobs":{"local":2}}`)
	result, err := handleCheckEngineHealth(context.Background(), map[string]string{"port": port})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !result.Success || result.Details["known"] != "true" || result.Details["healthy"] != "true" {
		t.Fatalf("expected known+healthy, got success=%v details=%v", result.Success, result.Details)
	}
}

// TestCheckEngineHealthHalfInitialized 僵尸 JVM：maps/overview 500 + status=fail → known=true 但不健康，原因透传。
// TestCheckEngineHealthHalfInitialized: a zombie JVM answers 500 + status=fail -> known but unhealthy,
// with the engine reason propagated.
func TestCheckEngineHealthHalfInitialized(t *testing.T) {
	port := engineHealthServerPort(t, http.StatusInternalServerError, `{"status":"fail","message":"coordinatorService is null"}`)
	result, err := handleCheckEngineHealth(context.Background(), map[string]string{"port": port})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Success || result.Details["known"] != "true" || result.Details["healthy"] != "false" {
		t.Fatalf("expected known+unhealthy, got success=%v details=%v", result.Success, result.Details)
	}
	if !strings.Contains(result.Details["reason"], "coordinatorService is null") {
		t.Fatalf("expected engine reason, got %q", result.Details["reason"])
	}
}

// TestCheckEngineHealthFailBodyOn200 200 但响应体 status=fail 同样判为不健康。
// TestCheckEngineHealthFailBodyOn200: a 200 with a status=fail body still counts as unhealthy.
func TestCheckEngineHealthFailBodyOn200(t *testing.T) {
	port := engineHealthServerPort(t, http.StatusOK, `{"status":"fail","message":"not initialized yet"}`)
	result, err := handleCheckEngineHealth(context.Background(), map[string]string{"port": port})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Success || result.Details["known"] != "true" || result.Details["healthy"] != "false" {
		t.Fatalf("expected known+unhealthy, got success=%v details=%v", result.Success, result.Details)
	}
	if !strings.Contains(result.Details["reason"], "not initialized yet") {
		t.Fatalf("expected engine reason, got %q", result.Details["reason"])
	}
}

// TestCheckEngineHealthUnknown 网络不可达 / 404 → known=false，绝不能误报为不健康。
// TestCheckEngineHealthUnknown: unreachable port or 404 -> known=false, never a false unhealthy.
func TestCheckEngineHealthUnknown(t *testing.T) {
	// 连接拒绝：找一个空闲端口后不监听。
	// Connection refused: pick a free port and never listen on it.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	freePort := fmt.Sprintf("%d", l.Addr().(*net.TCPAddr).Port)
	_ = l.Close()

	result, err := handleCheckEngineHealth(context.Background(), map[string]string{"port": freePort})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.Details["known"] != "false" || result.Details["healthy"] != "false" {
		t.Fatalf("expected known=false, got details=%v", result.Details)
	}
}
